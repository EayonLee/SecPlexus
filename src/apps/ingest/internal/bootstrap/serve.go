// Package bootstrap 初始化 Ingest 运行依赖并管理生命周期。
package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/publish"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/source"
	"github.com/EayonLee/SecPlexus/src/packages/go/cache"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化解析流水线、事件发布与 HTTP 通道，并管理它们的生命周期。
func Serve(ctx context.Context, cfg config.Config) (outcome error) {
	if err := cfg.ValidateServe(); err != nil {
		return err
	}
	logger, closeLog, err := logging.Open(cfg.Logging, "ingest")
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, closeLog()) }()

	slog.SetDefault(logger)

	startup, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	// 先建发布器：它连通性失败必须在启动期暴露，好过第一条事件才发现丢数据。
	producer, err := publish.NewKafka(startup, publish.Options{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topic,
	})
	if err != nil {
		return err
	}

	defer producer.Close()

	client, err := openCache(startup, cfg.Cache)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, client.Close()) }()

	limiter, err := httpchannel.NewRedisLimiter(client, rateBurst(cfg.HTTP.RateBurst), ratePerSecond(cfg.HTTP.RatePerSec))
	if err != nil {
		return err
	}

	line := pipeline.New(pipeline.Options{
		Workers:       cfg.Pipeline.Workers,
		QueueDepth:    cfg.Pipeline.QueueDepth,
		BatchSize:     cfg.Pipeline.BatchSize,
		FlushInterval: cfg.Pipeline.FlushInterval,
	}, producer)

	pipelineCtx, stopPipeline := context.WithCancel(context.Background())

	// 通道单独一份取消信号：停机时要先停接收方，再排空流水线。
	// 共用一个 ctx 会让通道在流水线排空期间继续投递，撞上 ErrClosed。
	channelCtx, stopChannels := context.WithCancel(context.Background())
	defer stopChannels()

	pipelineDone := make(chan error, 1)
	go func() { pipelineDone <- line.Start(pipelineCtx) }()

	// 接入点注册表：第五组接入数据库加载；当前为空表，
	// 未注册令牌一律 401——这比接受未知设备的推送安全。
	registry := source.NewMemory()

	// 显式标记"已加载（当前为空）"：readiness 的语义是"初始化完成"，
	// 不是"有接入点"。不调用这一步，Loaded() 恒为假，进程永远 503。
	registry.Replace(nil)

	server := &handler.Server{
		Ready: func(ctx context.Context) error {
			if err := client.Ping(ctx).Err(); err != nil {
				return errors.New("cache is not ready")
			}

			// 没有接入点的进程不能算就绪：它接不了任何数据，
			// 报 200 只会让负载均衡把流量导给一个空壳。
			if !registry.Loaded() {
				return errors.New("source registry is not loaded")
			}

			return nil
		},
	}

	channels := router.Deps{
		Registry: registry,
		Limiter:  limiter,
		Pipeline: line,
		MaxBody:  maxBodyBytes(cfg.HTTP.MaxBodyBytes),
	}

	// syslog 与 Kafka 消费是后台通道：它们不挂在 HTTP 路由上，各自有监听/轮询循环。
	syslogChannel := syslogchannel.New(registry, line, cfg.Syslog.ReloadInterval, cfg.Syslog.BindHost)
	syslogDone := make(chan struct{})
	go func() { defer close(syslogDone); _ = syslogChannel.Start(channelCtx) }()

	kafkaConsumer := kafkachannel.New(registry, line, producer, kafkachannel.Options{
		ReloadInterval: cfg.Pipeline.ReloadInterval,
	})
	kafkaDone := make(chan struct{})
	go func() { defer close(kafkaDone); _ = kafkaConsumer.Start(channelCtx) }()

	httpServer := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.RootHandler(server, channels),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr(),
		"workers", cfg.Pipeline.Workers, "topic", cfg.Kafka.Topic)

	serveErr := httpx.Serve(ctx, httpServer)

	// 停机顺序必须显式：停接收 → 停后台通道 → 排空流水线。
	// 之前 cancel 在 defer 里，而这里先阻塞等 pipelineDone，
	// 两者互相等待，进程收到 SIGTERM 后永远不退出。
	stopChannels()
	<-syslogDone
	<-kafkaDone
	stopPipeline()

	if err := <-pipelineDone; err != nil {
		logging.From(ctx).Error("Pipeline stopped with error", "event", "pipeline.stopped", "error", logging.Error(err))
	}

	if serveErr != nil {
		logging.From(ctx).Error("Could not stop HTTP server", "event", "service.listener_failed", "error", logging.Error(serveErr))
		return serveErr
	}

	return nil
}

// openCache 创建并验证 Redis 连接；失败时不遗留客户端连接。
func openCache(ctx context.Context, cfg cache.Config) (*redis.Client, error) {
	client, err := cache.Open(ctx, cfg)
	if err != nil {
		logging.From(ctx).Error("Redis connection failed", "event", "dependency.failed",
			"dependency", "redis", "error", logging.Error(err))

		return nil, err
	}

	return client, nil
}

// 默认值集中在这里：配置零值表示"用推荐的默认"。
const (
	defaultRatePerSec   = 50
	defaultRateBurst    = 200
	defaultMaxBodyBytes = 10 << 20
)

func ratePerSecond(value float64) float64 {
	if value <= 0 {
		return defaultRatePerSec
	}

	return value
}

func rateBurst(value int) int {
	if value <= 0 {
		return defaultRateBurst
	}

	return value
}

func maxBodyBytes(value int64) int64 {
	if value <= 0 {
		return defaultMaxBodyBytes
	}

	return value
}
