package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/bootstrap"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/config"
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
	var printEndpoints bool
	action := func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(filename)
		if err != nil {
			return err
		}
		if cmd.Name() == "check-config" {
			if err := cfg.ValidateServe(); err != nil {
				return err
			}
			if !printEndpoints {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Console configuration valid")
				return err
			}
		}
		if printEndpoints {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"public_addr": cfg.App.Addr()})
		}
		return bootstrap.Serve(cmd.Context(), cfg)
	}
	root := &cobra.Command{Use: "console", Short: "SecPlexus security operations service", Args: cobra.NoArgs, RunE: action, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&filename, "config", "", "Console YAML configuration file")
	root.PersistentFlags().BoolVar(&printEndpoints, "print-endpoints", false, "print effective endpoints without connecting dependencies")
	root.AddCommand(&cobra.Command{Use: "serve", Args: cobra.NoArgs, RunE: action})
	root.AddCommand(&cobra.Command{Use: "check-config", Args: cobra.NoArgs, RunE: action})
	return root
}
