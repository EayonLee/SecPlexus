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
	logger, closeLog, err := logging.Open(cfg.Logging, "console")
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, closeLog()) }()

	slog.SetDefault(logger)
	credential := cfg.IAM.Token
	client, err := iamclient.New(cfg.IAM.BaseURL, credential)
	if err != nil {
		return err
	}

	defer client.Close()

	// 启动期校验动作键还在 IAM 目录里：缺失时请求会在运行期静默 403。
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStartup()

	if err = client.ValidateActions(startupCtx, iamclient.PermissionConsoleContextRead); err != nil {
		// 目录漂移是配置错误，必须停在启动期；IAM 暂时不可达则先启动，
		// 请求会因为拿不到授权结论返回 503，依赖恢复后自动可用。
		if errors.Is(err, iamclient.ErrCatalogDrift) {
			return err
		}

		logging.From(context.Background()).Warn("Could not verify the permission catalog at startup",
			"event", "iam.catalog_unverified", "error", logging.Error(err))
	}

	server := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.Handler(&handler.Server{IAM: client}, client),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr())
	if err := httpx.Serve(ctx, server); err != nil {
		logging.From(ctx).Error("Could not stop HTTP server", "event", "service.listener_failed", "error", logging.Error(err))
		return err
	}
	return nil
}
