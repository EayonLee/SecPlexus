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

	// 权限目录、内置角色与配置管理员都在启动时幂等写入，前端只读这些内置数据。
	if err = store.Seed(startup, cfg.Bootstrap.Username, cfg.Bootstrap.Password); err != nil {
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

	limiter, err := authentication.NewLimiter(client, cfg.Auth.LoginIPRate, cfg.Auth.LoginAccountRate)
	if err != nil {
		return err
	}
	server, err := handler.New(&service.Service{
		Store:    store,
		Sessions: sessions,
		Config:   cfg,
	}, security, limiter, func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			logging.From(ctx).Error("PostgreSQL is not ready", "event", "dependency.unready", "dependency", "postgres", "error", logging.Error(err))
			return err
		}

		if err := client.Ping(ctx).Err(); err != nil {
			logging.From(ctx).Error("Redis is not ready", "event", "dependency.unready", "dependency", "redis", "error", logging.Error(err))
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

	if len(cfg.App.TrustedProxyCIDRs) == 0 {
		// 不是错误，但部署形态很容易踩：反代之后所有请求的来源都是反代地址，
		// 登录限流会退化成全站共用一个桶。启动时提示一次，让运维在日志里看到。
		slog.Info("No trusted proxy configured; client address comes from the direct peer",
			"event", "service.proxy_trust_disabled",
			"hint", "Set SPX_IAM_TRUSTED_PROXY_CIDRS when a reverse proxy forwards client addresses")
	}

	slog.Info("Starting HTTP listener", "event", "service.starting", "addr", cfg.App.Addr())
	if err := httpx.Serve(ctx, httpServer); err != nil {
		logging.From(ctx).Error("Could not stop HTTP server", "event", "service.listener_failed", "error", logging.Error(err))
		return err
	}
	return nil
}
