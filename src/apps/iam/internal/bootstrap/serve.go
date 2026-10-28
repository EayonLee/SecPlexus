package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 IAM 运行依赖，并负责 HTTP 服务的启动与关闭。
func Serve(ctx context.Context, cfg config.Config) (outcome error) {
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

	defer func() { outcome = errors.Join(outcome, store.Close()) }()

	if err = store.VerifySchema(startup); err != nil {
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
	group, serving := errgroup.WithContext(ctx)
	public.BaseContext = func(net.Listener) context.Context { return serving }
	internal.BaseContext = func(net.Listener) context.Context { return serving }
	group.Go(func() error {
		err := public.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		err := internal.ListenAndServeTLS(cfg.Internal.TLSCertFile, cfg.Internal.TLSKeyFile)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		<-serving.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := errors.Join(public.Shutdown(shutdown), internal.Shutdown(shutdown))
		if err != nil {
			err = errors.Join(err, public.Close(), internal.Close())
		}
		return err
	})
	slog.Info("HTTP 监听启动中", "event", "service.starting", "public_addr", cfg.App.Addr(), "internal_addr", cfg.Internal.Addr())
	if err := group.Wait(); err != nil {
		logging.From(ctx).Error("HTTP 服务关闭失败", "event", "service.listener_failed", "error", logging.Error(err))
		return errors.New("IAM HTTP listener failed")
	}
	return nil
}
