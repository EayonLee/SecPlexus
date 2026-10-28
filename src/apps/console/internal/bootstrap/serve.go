package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Serve 初始化 Console 所需的客户端和 HTTP 服务，并管理它们的生命周期。
func Serve(cfg config.Config) error {
	logger, closeLog, err := logging.Open(cfg.Logging, cfg.App.Name)
	if err != nil {
		return err
	}

	defer closeLog()

	slog.SetDefault(logger)
	credential := cfg.IAM.Token
	client, err := iamclient.New(cfg.IAM.BaseURL, credential, cfg.IAM.CAFile)
	if err != nil {
		return err
	}

	defer client.Close()

	server := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           (&handler.Server{IAM: client}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	failures := make(chan error, 1)
	go func() { failures <- server.ListenAndServe() }()
	slog.Info("HTTP 监听启动中", "event", "service.starting", "addr", cfg.App.Addr())
	select {
	case <-ctx.Done():
	case err = <-failures:
	}

	slog.Info("正在关闭 HTTP 服务", "event", "service.stopping")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if stopErr := server.Shutdown(shutdown); stopErr != nil {
		return stopErr
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP 监听异常结束", "event", "service.listener_failed", "error", logging.Error(err))
		return errors.New("Console HTTP listener failed")
	}

	return nil
}
