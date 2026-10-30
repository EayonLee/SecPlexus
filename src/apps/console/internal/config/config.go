// Package config 只负责 Console 配置加载和校验，不创建运行资源。
package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// App 描述 Console 服务自身的监听配置。
type App struct {
	Host string `yaml:"host" validate:"required,ip|eq=localhost"` // HTTP 监听地址。
	Port int    `yaml:"port" validate:"gte=1,lte=65535"`          // HTTP 监听端口。
}

// Addr 返回 HTTP 监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Config 包含 Console 启动和运行所需的配置。
type Config struct {
	App App `yaml:"app"` // 服务监听配置。
	IAM struct {
		Host    string `yaml:"host" validate:"required,hostname|ip"` // IAM 连接目标，IP 或域名。
		Port    int    `yaml:"port" validate:"gte=1,lte=65535"`      // IAM 连接端口。
		BaseURL string `yaml:"-"`                                    // 由结构化连接字段派生，不接受另一份 URL 配置。
		Token   string `yaml:"token"`                                // 调用 IAM 的服务凭证。
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
	if err := configuration.Decode(settings, &c); err != nil {
		return c, err
	}

	if err := configuration.Validate(c); err != nil {
		return c, err
	}

	if c.IAM.Host == "0.0.0.0" || c.IAM.Host == "::" {
		return c, errors.New("iam.host (SPX_CONSOLE_IAM_HOST / SPX_IAM_HOST) cannot be a wildcard")
	}
	c.IAM.BaseURL = (&url.URL{Scheme: "http", Host: net.JoinHostPort(c.IAM.Host, strconv.Itoa(c.IAM.Port))}).String()
	if err := iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, false); err != nil {
		return c, errors.New("iam.token (SPX_CONSOLE_IAM_TOKEN) must contain 32–512 bytes")
	}
	return c, c.Logging.Validate()
}

// ValidateServe 校验运行服务的完整凭证，必须在日志与客户端初始化之前调用。
func (c Config) ValidateServe() error {
	if err := iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, true); err != nil {
		return errors.New("iam.token (SPX_CONSOLE_IAM_TOKEN) must contain 32–512 bytes")
	}
	return nil
}
