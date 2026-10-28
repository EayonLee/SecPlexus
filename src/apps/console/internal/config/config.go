// Package config 只负责 Console 配置加载和校验，不创建运行资源。
package config

import (
	"errors"
	"net"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/packages/go/configfile"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// App 描述 Console 服务自身的监听配置。
type App struct {
	Name string `yaml:"name"` // 服务名称，用于日志标识。
	Host string `yaml:"host"` // HTTP 监听地址。
	Port int    `yaml:"port"` // HTTP 监听端口。
}

// Addr 返回 HTTP 监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Config 包含 Console 启动和运行所需的配置。
type Config struct {
	App App `yaml:"app"` // 服务监听配置。
	IAM struct {
		BaseURL string `yaml:"base_url"` // IAM 内部接口地址。
		Token   string `yaml:"token"`    // 调用 IAM 的服务凭证。
		CAFile  string `yaml:"ca_file"`  // 验证 IAM TLS 证书使用的 CA 文件。
	} `yaml:"iam"` // IAM 客户端配置。
	Logging logging.Config `yaml:"logging"` // 日志输出与轮转配置。
}

// Load 加载并校验配置文件；文件名为空时读取当前目录的 config.yaml。
func Load(filename string) (Config, error) {
	if filename == "" {
		filename = "config.yaml"
	}

	var c Config
	if err := configfile.Load(filename, &c); err != nil {
		return c, err
	}

	if c.App.Name == "" || len(c.App.Name) > 64 || c.App.Port < 1 || c.App.Port > 65535 || (net.ParseIP(c.App.Host) == nil && c.App.Host != "localhost") {
		return c, errors.New("invalid Console app name, host or port")
	}

	return c, c.Logging.Validate()
}
