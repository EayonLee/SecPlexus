package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 Console 所需的客户端和 HTTP 服务，并管理它们的生命周期。
func Serve(ctx context.Context, cfg config.Config) (outcome error) {
	if err := cfg.ValidateServe(); err != nil {
		return err
	}
	logger, closeLog, err := logging.Open(cfg.Logging, cfg.App.Name)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, closeLog()) }()

	slog.SetDefault(logger)
	credential := cfg.IAM.Token
	client, err := iamclient.New(cfg.IAM.BaseURL, credential, cfg.IAM.CAFile)
	if err != nil {
		return err
	}

	defer client.Close()

	server := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.Handler(&handler.Server{IAM: client}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("HTTP 监听启动中", "event", "service.starting", "addr", cfg.App.Addr())
	if err := httpx.Serve(ctx, server); err != nil {
		logging.From(ctx).Error("HTTP 服务关闭失败", "event", "service.listener_failed", "error", logging.Error(err))
		return err
	}
	return nil
}
