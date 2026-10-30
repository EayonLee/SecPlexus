package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 IAM 运行依赖，并负责 HTTP 服务的启动与关闭。
func Serve(ctx context.Context, cfg config.Config) (outcome error) {
	if err := cfg.ValidateServe(); err != nil {
		return err
	}

	startup, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	pool, err := repository.Open(startup, cfg.DB.URL)
	if err != nil {
		return err
	}

	defer pool.Close()

	store, err := repository.NewStore(pool)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, store.Close()) }()

	if err = store.VerifySchema(startup); err != nil {
		return err
	}

	if err = store.SyncAdministrator(startup, cfg.Bootstrap.Username, cfg.Bootstrap.Password); err != nil {
		return err
	}

	client, err := openCache(startup, cfg.Cache)
	if err != nil {
		return err
	}

	defer func() { outcome = errors.Join(outcome, client.Close()) }()

	sessions := authentication.NewSessions(store, cfg)
	security, err := iammiddleware.NewSecurity(cfg)
	if err != nil {
		return err
	}

	limiter, err := authentication.NewLimiter(client)
	if err != nil {
		return err
	}
	server, err := handler.New(&service.Service{
		Store:    store,
		Sessions: sessions,
		Config:   cfg,
	}, security, limiter, func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			logging.From(ctx).Error("PostgreSQL 就绪检查失败", "event", "dependency.unready", "dependency", "postgres", "error", logging.Error(err))
			return err
		}

		if err := client.Ping(ctx).Err(); err != nil {
			logging.From(ctx).Error("Redis 就绪检查失败", "event", "dependency.unready", "dependency", "redis", "error", logging.Error(err))
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.Handler(server),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("HTTP 监听启动中", "event", "service.starting", "addr", cfg.App.Addr())
	if err := httpx.Serve(ctx, httpServer); err != nil {
		logging.From(ctx).Error("HTTP 服务关闭失败", "event", "service.listener_failed", "error", logging.Error(err))
		return err
	}
	return nil
}
