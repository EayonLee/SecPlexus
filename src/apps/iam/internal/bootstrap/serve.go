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
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 IAM 运行依赖，并负责 HTTP 服务的启动与关闭。
func Serve(ctx context.Context, cfg config.Config) error {
	if len(cfg.Internal.Clients) == 0 || cfg.Internal.TLSCertFile == "" || cfg.Internal.TLSKeyFile == "" {
		return errors.New("internal client configuration and TLS certificate/key are required")
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

	defer store.Close()

	if err = store.VerifySchema(startup); err != nil {
		return err
	}

	client, err := openCache(startup, cfg.Cache)
	if err != nil {
		return err
	}

	defer client.Close()

	sessions := authentication.NewSessions(store, cfg)
	security, err := iammiddleware.NewSecurity(cfg)
	if err != nil {
		return err
	}

	server, err := handler.New(&service.Service{
		Store:    store,
		Sessions: sessions,
		Config:   cfg,
	}, security, &authentication.Limiter{Client: client}, func(ctx context.Context) error {
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

	public := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           server.PublicHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	internal := &http.Server{
		Addr:              cfg.Internal.Addr(),
		Handler:           server.InternalHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	failures := make(chan error, 2)
	go func() { failures <- public.ListenAndServe() }()
	go func() { failures <- internal.ListenAndServeTLS(cfg.Internal.TLSCertFile, cfg.Internal.TLSKeyFile) }()
	slog.Info("HTTP 监听启动中", "event", "service.starting", "public_addr", cfg.App.Addr(), "internal_addr", cfg.Internal.Addr())
	select {
	case <-ctx.Done():
	case err = <-failures:
	}

	slog.Info("正在关闭 HTTP 服务", "event", "service.stopping")
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	publicErr := public.Shutdown(shutdown)
	internalErr := internal.Shutdown(shutdown)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP 监听异常结束", "event", "service.listener_failed", "error", logging.Error(err))
		return errors.New("IAM HTTP listener failed")
	}

	return errors.Join(publicErr, internalErr)
}
