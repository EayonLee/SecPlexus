// Package bootstrap 初始化 Ingest 运行依赖并管理生命周期。
package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/publish"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/cache"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
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
		Brokers:           cfg.Output.Kafka.Brokers,
		EventTopic:        cfg.Output.Kafka.EventTopic,
		ParseFailureTopic: cfg.Output.Kafka.ParseFailureTopic,
		Security: kafka.Security{
			Protocol:    cfg.Output.Kafka.SecurityProtocol,
			Mechanism:   cfg.Output.Kafka.SASLMechanism,
			Username:    cfg.Output.Kafka.SASLUsername,
			PasswordEnv: cfg.Output.Kafka.SASLPasswordEnv,
			CAFile:      cfg.Output.Kafka.TLSCAFile,
		},
	})
	if err != nil {
		return err
	}

	defer producer.Close()

	// 数据库是接入点与解析规则的来源；首次加载失败就启动失败——
	// 与 nginx 遇到坏配置拒绝启动同理：不知道自己收什么的服务不该对外报就绪。
	pool, err := repository.Open(startup, cfg.DB.DSN())
	if err != nil {
		return err
	}

	defer pool.Close()

	client, err := openCache(startup, cfg.Cache)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, client.Close()) }()

	limiter, err := httpchannel.NewRedisLimiter(client, rateBurst(cfg.Input.HTTP.RateBurst), ratePerSecond(cfg.Input.HTTP.RatePerSec))
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

	// 通道单独一份取消信号：停机时要先停接收方，再排空流水线。
	// 共用一个 ctx 会让通道在流水线排空期间继续投递，撞上 ErrClosed。
	channelCtx, stopChannels := context.WithCancel(context.Background())
	defer stopChannels()

	pipelineDone := make(chan error, 1)
	go func() { pipelineDone <- line.Start(pipelineCtx) }()

	// 接入点注册表：内容来自数据库，未注册令牌一律 401——
	// 这比接受未知设备的推送安全。
	registry := input.NewMemory()

	refresher := newSourceRefresher(pool, registry, cfg.Input.Kafka.ReloadInterval)
	if err = refresher.Load(startup); err != nil {
		return err
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()

	go refresher.Watch(watchCtx)

	server := &handler.Server{
		Stats: line.Stats,
		Ready: func(ctx context.Context) error {
			if err := client.Ping(ctx).Err(); err != nil {
				return errors.New("cache is not ready")
			}

			// 没有接入点的进程不能算就绪：它接不了任何数据，
			// 报 200 只会让负载均衡把流量导给一个空壳。
			if !registry.Loaded() {
				return errors.New("source registry is not loaded")
			}

			// 曾经加载成功、之后刷新失败时旧配置仍在服务：
			// 这种情况靠日志暴露，不再把实例摘掉，否则会中断正常工作的接入点。
			if degraded := refresher.Degraded(); degraded != "" {
				logging.From(ctx).Warn("Source refresh degraded", "event", "input.degraded", "reason", degraded)
			}

			return nil
		},
	}

	// 管理面的鉴权与审计都由 iamclient 完成，业务代码不含权限判断。
	iam, err := iamclient.New(cfg.IAM.BaseURL, cfg.IAM.Token)
	if err != nil {
		return err
	}

	defer iam.Close()

	channels := router.Deps{
		Registry: registry,
		Limiter:  limiter,
		Pipeline: line,
		MaxBody:  maxBodyBytes(cfg.Input.HTTP.MaxBodyBytes),
		Admin:    repository.NewAdminStore(pool),
		IAM:      iam,
	}

	// syslog 与 Kafka 消费是后台通道：它们不挂在 HTTP 路由上，各自有监听/轮询循环。
	syslogChannel := syslogchannel.New(registry, line, cfg.Input.Syslog.ReloadInterval, cfg.Input.Syslog.Host)
	syslogDone := make(chan struct{})
	go func() { defer close(syslogDone); _ = syslogChannel.Start(channelCtx) }()

	kafkaConsumer := kafkachannel.New(registry, line, producer, kafkachannel.Options{
		ReloadInterval: cfg.Input.Kafka.ReloadInterval,
		Security:       producerSecurity(cfg.Output.Kafka),
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
		"workers", cfg.Pipeline.Workers, "event_topic", cfg.Output.Kafka.EventTopic)

	// 配置了证书就启用 TLS；监听与停机路径与明文一致。
	if cfg.Input.HTTP.TLSCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.Input.HTTP.TLSCertFile, cfg.Input.HTTP.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("could not load TLS certificate: %w", err)
		}

		httpServer.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
		slog.Info("Starting HTTPS listener", "event", "service.starting_tls", "addr", cfg.App.Addr())
	}

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

// producerSecurity 把 Kafka 安全配置从配置结构转成客户端输入。
// 生产与消费共用一份：同一集群不应有两套凭据来源。
func producerSecurity(kafkaConfig config.Kafka) kafka.Security {
	return kafka.Security{
		Protocol:    kafkaConfig.SecurityProtocol,
		Mechanism:   kafkaConfig.SASLMechanism,
		Username:    kafkaConfig.SASLUsername,
		PasswordEnv: kafkaConfig.SASLPasswordEnv,
		CAFile:      kafkaConfig.TLSCAFile,
	}
}
