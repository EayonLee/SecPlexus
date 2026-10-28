// Command iam 选择身份服务、显式迁移和维护命令。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
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

// run 读取命令参数与配置，再选择启动或数据库维护入口。
func run() error {
	command := "serve"
	args := os.Args[1:]

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	flags := flag.NewFlagSet("iam", flag.ContinueOnError)
	filename := flags.String("config", "", "IAM YAML configuration file (default config.yaml)")
	printEndpoints := flags.Bool("print-endpoints", false, "print only effective listen endpoints for orchestration")
	userID := flags.String("user-id", "", "existing account UUID for operator password recovery")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected IAM arguments")
	}

	cfg, err := config.Load(*filename)

	if err != nil {
		return err
	}

	if *printEndpoints {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"public_addr": cfg.App.Addr(), "internal_addr": cfg.Internal.Addr()})
	}

	logger, closeLog, err := logging.Open(cfg.Logging, cfg.App.Name)

	if err != nil {
		return err
	}

	defer closeLog()

	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

		defer store.Close()

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

		defer store.Close()

		switch command {
		case "maintain":
			return store.Maintain(maintenance, cfg.Retention.AuditDays, cfg.Retention.SessionDays)
		case "revoke-after-restore":
			return store.RevokeAfterRestore(maintenance)
		case "reset-password":
			id, err := uuid.Parse(*userID)
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

	if command != "serve" {
		return errors.New("supported IAM commands: serve, migrate, maintain, revoke-after-restore, reset-password")
	}

	return bootstrap.Serve(ctx, cfg)
}
