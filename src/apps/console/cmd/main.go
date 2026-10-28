package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

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

// run 加载配置，输出有效端点或启动 Console 服务。
func run() error {
	filename := flag.String("config", "", "explicit Console YAML configuration file")
	printEndpoints := flag.Bool("print-endpoints", false, "print only effective listen endpoints for orchestration")
	flag.Parse()
	cfg, err := config.Load(*filename)
	if err != nil {
		return err
	}

	if *printEndpoints {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"public_addr": cfg.App.Addr()})
	}

	return bootstrap.Serve(cfg)
}
