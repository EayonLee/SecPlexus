// Package bootstrap 初始化 Ingest 运行依赖并管理生命周期。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/kafkachannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/syslogchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/publish"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	redisconn "github.com/EayonLee/SecPlexus/src/packages/go/redis"
)

// Serve 初始化解析流水线、事件发布与 HTTP 通道，并管理它们的生命周期。
func Serve(ctx context.Context, cfg config.Config) (outcome error) {
	tlsConfig, err := cfg.ValidateServe()
	if err != nil {
		return err
	}
	catalog, err := events.Load()
	if err != nil {
		return fmt.Errorf("load event schema: %w", err)
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
		Brokers:    cfg.Kafka.Brokers,
		EventTopic: cfg.Events.Topic,

		Security: cfg.Kafka.Security(cfg.Events.Publisher),
	})
	if err != nil {
		return err
	}

	defer producer.Close()

	// 数据库是接入点与解析规则的来源；首次加载失败就启动失败——
	// 与 nginx 遇到坏配置拒绝启动同理：不知道自己收什么的服务不该对外报就绪。
	pool, err := repository.Open(startup, cfg.PostgreSQL.DSN())
	if err != nil {
		return err
	}

	defer pool.Close()
	if err := repository.VerifySchema(startup, pool); err != nil {
		return err
	}

	client, err := openRedis(startup, cfg.Redis)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, client.Close()) }()

	limiter, err := httpchannel.NewRedisLimiter(client, rateBurst(cfg.Input.HTTP.RateBurst), ratePerSecond(cfg.Input.HTTP.RatePerSec))
	if err != nil {
		return err
	}

	observations := observation.New(client)
	observationCtx, cancelObservations := context.WithCancel(context.Background())
	observationDone := make(chan struct{})
	go func() {
		defer close(observationDone)
		observations.Run(observationCtx)
	}()
	defer func() {
		cancelObservations()
		<-observationDone
	}()
	line := pipeline.New(pipeline.Options{
		Observations:     observations,
		Workers:          cfg.Pipeline.Workers,
		QueueDepth:       cfg.Pipeline.QueueDepth,
		MaxInFlightBytes: cfg.Pipeline.MaxInFlightBytes,
		BatchSize:        cfg.Pipeline.BatchSize,
		FlushInterval:    cfg.Pipeline.FlushInterval,
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

	syslogChannel := syslogchannel.New(registry, line, cfg.Input.Syslog.ReloadInterval, cfg.Input.Syslog.Host)

	kafkaConsumer := kafkachannel.New(registry, line, producer, kafkachannel.Options{
		Execution:      repository.NewKafkaExecutionStore(pool),
		ReloadInterval: cfg.Input.Kafka.ReloadInterval,
		Credentials:    cfg.Input.Kafka.Credentials,
	})

	var httpOnline atomic.Bool
	channelActivation := func(entry input.Entry) input.ChannelActivation {
		switch entry.Protocol {
		case input.ProtocolHTTP:
			return input.ChannelActivation{Online: httpOnline.Load() && entry.HasToken()}
		case input.ProtocolSyslog:
			return input.ChannelActivation{Online: syslogChannel.Ready(entry)}
		case input.ProtocolKafka:
			return kafkaConsumer.Activation(entry)
		default:
			return input.ChannelActivation{}
		}
	}
	refresher := newSourceRefresher(pool, registry, observations, channelActivation, repository.ReloadInterval)

	if err = refresher.Load(startup); err != nil {
		return err
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()

	go refresher.Watch(watchCtx)

	server := &handler.Server{
		Stats:                line.Stats,
		SourceSnapshotAge:    refresher.SnapshotAge,
		KafkaOffsetsRejected: kafkaConsumer.OffsetsRejected,
		Ready: func(ctx context.Context) error {
			if err := client.Ping(ctx).Err(); err != nil {
				return errors.New("cache is not ready")
			}

			// 没有接入点的进程不能算就绪：它接不了任何数据，
			// 报 200 只会让负载均衡把流量导给一个空壳。
			if !registry.Loaded() {
				return errors.New("source registry is not loaded")
			}

			// 权威数据源故障时只在快照租期内继续受理，过期由 Loaded 拒绝就绪。
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

	// 启动期校验动作键还在 IAM 目录里：缺失时管理请求会在运行期静默 403。
	if err = iam.ValidateActions(startup, ingestActions...); err != nil {
		// 目录漂移是配置错误，必须停在启动期；IAM 暂时不可达则先启动，
		// 请求会因为拿不到授权结论返回 503，依赖恢复后自动可用。
		if errors.Is(err, iamclient.ErrCatalogDrift) {
			return err
		}

		logging.From(context.Background()).Warn("Could not verify the permission catalog at startup",
			"event", "iam.catalog_unverified", "error", logging.Error(err))
	}

	adminStore := repository.NewAdminStore(pool)
	server.AuditOutboxHealth = adminStore.AuditOutboxHealth
	auditCtx, stopAudit := context.WithCancel(ctx)
	auditDone := make(chan struct{})
	go func() {
		defer close(auditDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-auditCtx.Done():
				return
			case <-ticker.C:
				delivery, cancel := context.WithTimeout(auditCtx, 5*time.Second)
				deliverErr := adminStore.DeliverAudit(delivery, iam.RecordAudit)
				cancel()
				if deliverErr != nil && auditCtx.Err() == nil {
					logging.From(auditCtx).Error("Could not deliver pending audit", "event", "audit.delivery_failed", "error", logging.Error(deliverErr))
				}
			}
		}
	}()
	defer func() {
		stopAudit()
		<-auditDone
	}()

	push := httpchannel.New(registry, limiter, line)
	push.MaxBodyBytes = maxBodyBytes(cfg.Input.HTTP.MaxBodyBytes)
	push.MaxConcurrentRequests = cfg.Input.HTTP.MaxConcurrentRequests
	var readEvent gin.HandlerFunc
	if ch := cfg.ClickHouse; ch.URL != "" {
		reader, err := event.NewStore(ch.URL, ch.Database, ch.ReaderUsername, ch.ReaderPassword, ch.Timeout)
		if err != nil {
			return err
		}
		defer reader.Close()
		readEvent = handler.ReadEvent(reader)
	}
	routes := router.Deps{
		EventRead:   readEvent,
		Push:        handler.DevicePush(push),
		EventSchema: &handler.EventSchema{Catalog: catalog},
		Admin: &handler.Admin{Store: adminStore, Observations: observations, KafkaCredentialAllowed: func(name string, brokers []string) bool {
			profile, exists := cfg.Input.Kafka.Credentials[name]
			return exists && profile.Allows(brokers)
		}},
		IAM: iam,
	}

	// syslog 与 Kafka 消费是后台通道：它们不挂在 HTTP 路由上，各自有监听/轮询循环。
	syslogDone := make(chan struct{})
	go func() { defer close(syslogDone); _ = syslogChannel.Start(channelCtx) }()

	kafkaDone := make(chan struct{})
	go func() { defer close(kafkaDone); _ = kafkaConsumer.Start(channelCtx) }()

	routeHandler := router.Handler(server, routes)
	httpServer := &http.Server{
		Addr: cfg.App.Addr(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpOnline.Store(true)
			routeHandler.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
		TLSConfig:         tlsConfig,
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr(),
		"workers", cfg.Pipeline.Workers, "event_topic", cfg.Events.Topic)

	// 配置了证书就启用 TLS；监听与停机路径与明文一致。
	if tlsConfig != nil {
		slog.Info("Starting HTTPS listener", "event", "service.starting_tls", "addr", cfg.App.Addr())
	}

	serveErr := httpx.Serve(ctx, httpServer)
	httpOnline.Store(false)
	stopWatch()

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

// openRedis 创建并验证 Redis 连接；失败时不遗留客户端连接。
func openRedis(ctx context.Context, cfg redisconn.Config) (*redis.Client, error) {
	client, err := redisconn.Open(ctx, cfg)
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

// ingestActions 是 Ingest 依赖的权限键：新增路由时必须同步加入。
var ingestActions = []string{
	iamclient.PermissionIngestEventsRead,
	iamclient.PermissionIngestInputRead,
	iamclient.PermissionIngestInputWrite,
	iamclient.PermissionIngestParsingRuleRead,
	iamclient.PermissionIngestParsingRuleWrite,
}
