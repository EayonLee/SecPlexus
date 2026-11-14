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
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/publish"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
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
	defer stopPipeline()

	pipelineDone := make(chan error, 1)
	go func() { pipelineDone <- line.Start(pipelineCtx) }()

	// 接入点注册表：第五组接入数据库加载；当前为空表，
	// 未注册令牌一律 401——这比接受未知设备的推送安全。
	registry := connector.NewMemory()

	server := &handler.Server{
		Registry: registry,
		Pipeline: line,
		Ready: func(ctx context.Context) error {
			if err := client.Ping(ctx).Err(); err != nil {
				return errors.New("cache is not ready")
			}

			return nil
		},
	}

	channels := router.Channels{
		Registry: registry,
		Limiter:  limiter,
		Pipeline: line,
		MaxBody:  maxBodyBytes(cfg.HTTP.MaxBodyBytes),
	}

	// syslog 与 Kafka 消费是后台通道：它们不挂在 HTTP 路由上，各自有监听/轮询循环。
	syslogChannel := syslogchannel.New(registry, line, cfg.Syslog.ReloadInterval)
	syslogDone := make(chan struct{})
	go func() { defer close(syslogDone); _ = syslogChannel.Start(pipelineCtx) }()

	kafkaConsumer := kafkachannel.New(registry, line, producer, kafkachannel.Options{
		ReloadInterval: cfg.Pipeline.ReloadInterval,
	})
	kafkaDone := make(chan struct{})
	go func() { defer close(kafkaDone); _ = kafkaConsumer.Start(pipelineCtx) }()

	defer func() {
		// 通道先停：不再产生新报文，之后的排空才有意义。
		stopPipeline()
		<-syslogDone
		<-kafkaDone
	}()

	httpServer := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.Handler(server, channels),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr(),
		"workers", cfg.Pipeline.Workers, "topic", cfg.Kafka.Topic)

	serveErr := httpx.Serve(ctx, httpServer)

	// 停机顺序：停接收（HTTP 与后台通道）→ 排空在途解析 → 发布剩余批次。
	// 后台通道的停止由上面的 defer 完成；此处等流水线跑完。
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
	options, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, errors.New("invalid cache connection configuration")
	}

	options.DialTimeout = 3 * time.Second
	options.ReadTimeout = 2 * time.Second
	options.WriteTimeout = 2 * time.Second
	options.PoolTimeout = 3 * time.Second
	options.MaxRetries = -1

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		logging.From(ctx).Error("Redis connection failed", "event", "dependency.failed",
			"dependency", "redis", "error", logging.Error(err))

		return nil, errors.New("cache unavailable")
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
