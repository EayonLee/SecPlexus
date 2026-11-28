// Command ingest 启动安全事件管道及其维护命令。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/bootstrap"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
)

// main 处理进程退出码，具体初始化和命令执行交给 run。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 使用 Cobra 处理参数，在运行前加载独立的模块配置。
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return newCommand().ExecuteContext(ctx)
}

// newCommand 创建不持有连接的独立命令树。
func newCommand() *cobra.Command {
	var filename string
	var printEndpoints, storeRole bool
	action := func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(filename)
		if err != nil {
			return err
		}
		if cmd.Name() == "migrate" {
			if err := cfg.PostgreSQL.Validate(); err != nil {
				return err
			}
			migrationCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			return repository.Migrate(migrationCtx, cfg.PostgreSQL.DSN())
		}
		if cmd.Name() == "store-events" {
			return bootstrap.StoreEvents(cmd.Context(), cfg)
		}
		if cmd.Name() == "check-store-config" {
			if err := cfg.ValidateStoreEvents(); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Event storage configuration valid")
			return err
		}
		if cmd.Name() == "check-event-store" || cmd.Name() == "migrate-event-store" {
			options := cfg.ClickHouse
			if err := options.Validate(); err != nil {
				return err
			}
			username, password := options.Username, options.Password
			if cmd.Name() == "migrate-event-store" {
				username, password = options.MigrationUsername, options.MigrationPassword
			}
			store, err := event.NewStore(options.URL, options.Database, username, password, options.Timeout)
			if err != nil {
				return err
			}
			defer store.Close()
			if cmd.Name() == "migrate-event-store" {
				return store.Migrate(cmd.Context())
			}
			if err := store.CheckSchema(cmd.Context()); err != nil {
				return err
			}
			if err := store.CheckPermissions(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "ClickHouse event schema and permissions are valid")
			return err
		}
		if cmd.Name() == "check-kafka" || cmd.Name() == "init-event-topics" {
			identity := cfg.Events.Publisher
			var failureTopic, group string
			if storeRole {
				identity = cfg.Events.Consumer
				failureTopic, group = cfg.Events.FailureTopic, cfg.Events.ConsumerGroup
				if failureTopic == "" || group == "" {
					return fmt.Errorf("event storage requires a failure topic and consumer group")
				}
			}
			if err := cfg.ValidateKafka(identity); err != nil {
				return err
			}
			retentionMS, _ := cfg.Kafka.RetentionMilliseconds()
			check, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			if cmd.Name() == "init-event-topics" {
				return kafka.InitEventTopics(check, cfg.Kafka.Brokers, cfg.Kafka.Security(identity),
					cfg.Events.Topic, cfg.Events.FailureTopic, retentionMS)
			}
			return kafka.CheckEvents(check, cfg.Kafka.Brokers, cfg.Kafka.Security(identity),
				cfg.Events.Topic, failureTopic, group, retentionMS)
		}
		if cmd.Name() == "check-config" {
			tlsConfig, err := cfg.ValidateServe()
			if err != nil {
				return err
			}
			if printEndpoints {
				scheme := "http"
				if tlsConfig != nil {
					scheme = "https"
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"public_addr": cfg.App.Addr(), "scheme": scheme})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Ingest configuration valid")
			return err
		}
		return bootstrap.Serve(cmd.Context(), cfg)
	}
	root := &cobra.Command{Use: "ingest", Short: "SecPlexus security event pipeline", Args: cobra.NoArgs, RunE: action, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&filename, "config", "", "Ingest YAML configuration file")
	root.PersistentFlags().BoolVar(&printEndpoints, "print-endpoints", false, "print effective endpoints without connecting dependencies")
	root.AddCommand(&cobra.Command{Use: "serve", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "check-config", Args: cobra.NoArgs, RunE: action})
	kafkaCheck := &cobra.Command{Use: "check-kafka", Short: "Check Kafka topic metadata, retention and permissions", Args: cobra.NoArgs, RunE: action}
	kafkaCheck.Flags().BoolVar(&storeRole, "store", false, "check write role, failure topic and event consumer group")
	root.AddCommand(kafkaCheck)
	root.AddCommand(&cobra.Command{Use: "init-event-topics", Short: "Create the internal Kafka event topics", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "migrate", Short: "Apply database migrations", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "store-events", Short: "Consume Kafka events and store them in ClickHouse", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "check-store-config", Short: "Validate event storage configuration without connecting", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "migrate-event-store", Short: "Initialize the versioned ClickHouse event schema", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "check-event-store", Short: "Check the deployed ClickHouse event table and permissions", Args: cobra.NoArgs, RunE: action})
	eventDDL := &cobra.Command{
		Use: "event-ddl", Short: "Render the ClickHouse event table migration", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sql, err := event.EventTableMigration()
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), sql)
			return err
		},
	}
	root.AddCommand(eventDDL)
	return root
}
