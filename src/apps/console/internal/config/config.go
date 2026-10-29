// Package config 只负责 Console 配置加载和校验，不创建运行资源。
package config

import (
	"errors"
	"net"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// App 描述 Console 服务自身的监听配置。
type App struct {
	Name string `yaml:"name"`                                     // 服务名称，用于日志标识。
	Host string `yaml:"host" validate:"required,ip|eq=localhost"` // HTTP 监听地址。
	Port int    `yaml:"port" validate:"gte=1,lte=65535"`          // HTTP 监听端口。
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
	settings, err := configuration.Read(filename)
	if err != nil {
		return c, err
	}
	bindEnvironment(settings)
	if err := configuration.Decode(settings, &c); err != nil {
		return c, err
	}

	if err := configuration.Validate(c); err != nil {
		return c, err
	}
	if c.App.Name == "" || len(c.App.Name) > 64 {
		return c, errors.New("invalid console service name")
	}

	if err := iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, false); err != nil {
		return c, err
	}
	return c, c.Logging.Validate()
}

// ValidateServe 校验运行服务的完整凭证，必须在日志与客户端初始化之前调用。
func (c Config) ValidateServe() error {
	return iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, true)
}
