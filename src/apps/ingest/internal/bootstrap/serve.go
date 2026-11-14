// Package bootstrap 初始化 Ingest 运行依赖并管理生命周期。
package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 Ingest 所需的客户端和 HTTP 服务，并管理它们的生命周期。
// 骨架阶段只验证配置、建立 IAM 客户端并监听就绪检查；
// Kafka 生产者、Redis 限流与三通道在后续批次接入。
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
	// 就绪检查集中在一个钩子上：依赖落地一批，探活就补一批，
	// 避免"进程活着但依赖全挂仍然接流量"的可用性事故。
	server := &handler.Server{
		Ready: func(ctx context.Context) error {
			// 骨架阶段没有常驻外部依赖；通道与发布层落地后在这里探活 Kafka 与 Redis。
			return nil
		},
	}

	httpServer := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.Handler(server),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr())
	if err := httpx.Serve(ctx, httpServer); err != nil {
		logging.From(ctx).Error("Could not stop HTTP server", "event", "service.listener_failed", "error", logging.Error(err))
		return err
	}
	return nil
}
