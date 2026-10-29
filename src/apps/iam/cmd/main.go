// Command iam 选择身份服务、显式迁移和维护命令。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/bootstrap"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// main 处理进程退出码，具体初始化和命令执行交给 run。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 通过 Cobra 执行命令，并向所有用例传递进程取消信号。
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return newCommand().ExecuteContext(ctx)
}

// newCommand 创建独立命令树，不持有全局配置或运行连接。
func newCommand() *cobra.Command {
	var filename, userID string
	var printEndpoints bool
	run := func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(filename)
		if err != nil {
			return err
		}
		if printEndpoints {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"public_addr": cfg.App.Addr(), "internal_addr": cfg.Internal.Addr()})
		}
		name := cmd.Name()
		if name == "iam" {
			name = "serve"
		}
		return execute(cmd.Context(), name, cfg, userID)
	}
	root := &cobra.Command{Use: "iam", Short: "SecPlexus identity service", Args: cobra.NoArgs, RunE: run, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&filename, "config", "", "IAM YAML configuration file")
	root.PersistentFlags().BoolVar(&printEndpoints, "print-endpoints", false, "print effective endpoints without connecting dependencies")
	for _, name := range []string{"serve", "migrate", "status", "maintain", "revoke-after-restore", "reset-password"} {
		command := &cobra.Command{Use: name, Args: cobra.NoArgs, RunE: run}
		if name == "reset-password" {
			command.Flags().StringVar(&userID, "user-id", "", "existing account UUID")
			_ = command.MarkFlagRequired("user-id")
			command.Args = func(cmd *cobra.Command, args []string) error {
				if err := cobra.NoArgs(cmd, args); err != nil {
					return err
				}
				if _, err := uuid.Parse(userID); err != nil {
					return errors.New("--user-id must identify an existing account")
				}
				return nil
			}
		}
		root.AddCommand(command)
	}
	return root
}

// execute 在命令参数和配置验证后创建用例实际需要的资源。
func execute(ctx context.Context, command string, cfg config.Config, userID string) (outcome error) {
	if command == "serve" {
		if err := cfg.ValidateServe(); err != nil {
			return err
		}
	}
	logger, closeLog, err := logging.Open(cfg.Logging, cfg.App.Name)
	if err != nil {
		return err
	}
	defer func() { outcome = errors.Join(outcome, closeLog()) }()
	slog.SetDefault(logger)
	if command == "status" {
		return repository.Status(ctx, cfg.DB.URL, os.Stdout)
	}
	if command == "migrate" {
		migrationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err = repository.Migrate(migrationCtx, cfg.DB.URL); err != nil {
			return err
		}

		pool, err := repository.Open(migrationCtx, cfg.DB.URL)
		if err != nil {
			return err
		}

		defer pool.Close()

		store, err := repository.NewStore(pool)
		if err != nil {
			return err
		}

		defer func() { outcome = errors.Join(outcome, store.Close()) }()

		if err = store.Seed(migrationCtx, cfg.Bootstrap.Username, cfg.Bootstrap.Password); err != nil {
			return err
		}

		fmt.Println("IAM migrations and initialization completed.")
		return nil
	}

	if command == "maintain" || command == "revoke-after-restore" || command == "reset-password" {
		maintenance, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		pool, err := repository.Open(maintenance, cfg.DB.URL)
		if err != nil {
			return err
		}

		defer pool.Close()

		store, err := repository.NewStore(pool)
		if err != nil {
			return err
		}

		defer func() { outcome = errors.Join(outcome, store.Close()) }()

		switch command {
		case "maintain":
			return store.Maintain(maintenance, cfg.Retention.AuditDays, cfg.Retention.SessionDays)
		case "revoke-after-restore":
			return store.RevokeAfterRestore(maintenance)
		case "reset-password":
			id, err := uuid.Parse(userID)
			if err != nil {
				return errors.New("--user-id must identify an existing account")
			}
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("password recovery requires an interactive terminal")
			}
			_, _ = fmt.Fprint(os.Stderr, "New password: ")

			first, err := term.ReadPassword(int(os.Stdin.Fd()))
			_, _ = fmt.Fprintln(os.Stderr)

			if err != nil {
				return errors.New("cannot read password")
			}
			_, _ = fmt.Fprint(os.Stderr, "Confirm password: ")

			second, err := term.ReadPassword(int(os.Stdin.Fd()))
			_, _ = fmt.Fprintln(os.Stderr)

			if err != nil {
				return errors.New("cannot read password")
			}

			if string(first) != string(second) {
				return errors.New("passwords do not match")
			}

			err = store.RecoverPassword(maintenance, id, string(first))

			clear(first)
			clear(second)

			return err
		}
	}

	return bootstrap.Serve(ctx, cfg)
}
