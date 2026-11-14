// Package config 只负责 Ingest 配置加载和校验，不创建运行资源。
package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/cache"
	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// App 描述 Ingest 的统一监听配置。
// 可信代理网段在 HTTP 通道（按来源 IP 限流）落地时随路由布线一起增加，对齐 iam 的消费方式。
type App struct {
	Host string `yaml:"host" validate:"required,ip|eq=localhost"` // HTTP 监听地址。
	Port int    `yaml:"port" validate:"gte=1,lte=65535"`          // HTTP 监听端口。
}

// Addr 返回 HTTP 服务的监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Kafka 描述事件发布总线；Kafka 是第一版唯一缓冲层（不落盘原则）。
type Kafka struct {
	Brokers []string `yaml:"brokers" validate:"dive,hostname_port"` // broker 地址列表；serve 必填。
	Topic   string   `yaml:"topic" validate:"required"`             // 唯一发布 topic；解析失败事件也发这里（parse_status 区分）。
}

// Pipeline 描述解析流水线的容量参数；零值由管道填默认值，配置里只写想改的。',
type Pipeline struct {
	Workers        int           `yaml:"workers"`         // 解析 worker 数；0 表示按 CPU 核数。
	QueueDepth     int           `yaml:"queue_depth"`     // 待解析队列长度；0 表示 10000，队列满时阻塞接收。
	BatchSize      int           `yaml:"batch_size"`      // 每批事件数；0 表示 500。
	ReloadInterval time.Duration `yaml:"reload_interval"` // Kafka 接入点刷新间隔；0 表示 30 秒。
	FlushInterval  time.Duration `yaml:"flush_interval"`  // 批量最长等待；0 表示 2 秒。
}

// Syslog 描述 syslog 通道参数。
type Syslog struct {
	// ReloadInterval 是端口绑定关系的刷新间隔；SIGHUP 可立即重载。
	ReloadInterval time.Duration `yaml:"reload_interval"`
}

// HTTPChannel 描述设备推送通道的参数。
type HTTPChannel struct {
	MaxBodyBytes int64   `yaml:"max_body_bytes"` // 请求体上限，超出返回 413；0 表示 10 MiB。
	RatePerSec   float64 `yaml:"rate_per_sec"`   // 每接入点每秒请求配额；0 表示 50。
	RateBurst    int     `yaml:"rate_burst"`     // 每接入点突发额度；0 表示 200。
}

// IAM 描述授权上游；Ingest 的管理 API 每个受保护请求都向 IAM 换取授权结论。
type IAM struct {
	Host    string `yaml:"host" validate:"required,hostname|ip"` // IAM 连接目标，IP 或域名。
	Port    int    `yaml:"port" validate:"gte=1,lte=65535"`      // IAM 连接端口。
	BaseURL string `yaml:"-"`                                    // 由结构化连接字段派生，不接受另一份 URL 配置。
	Token   string `yaml:"token"`                                // 调用 IAM 的服务凭证。
}

// Config 包含 Ingest 启动和运行所需的配置。
type Config struct {
	App      App            `yaml:"app"`          // 服务监听配置。
	Pipeline Pipeline       `yaml:"pipeline"`     // 解析流水线容量。
	HTTP     HTTPChannel    `yaml:"http_channel"` // 设备推送通道参数。
	Syslog   Syslog         `yaml:"syslog"`       // syslog 通道参数。
	Kafka    Kafka          `yaml:"kafka"`        // 事件发布总线。
	Cache    cache.Config   `yaml:"cache"`        // 令牌限流用的 Redis。
	IAM      IAM            `yaml:"iam"`          // 管理 API 的授权上游。
	Logging  logging.Config `yaml:"logging"`      // 日志输出与轮转配置。
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

	if c.IAM.Host == "0.0.0.0" || c.IAM.Host == "::" {
		return c, errors.New("iam.host (SPX_IAM_HOST) cannot be a wildcard")
	}
	c.IAM.BaseURL = (&url.URL{Scheme: "http", Host: net.JoinHostPort(c.IAM.Host, strconv.Itoa(c.IAM.Port))}).String()
	if err := iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, false); err != nil {
		return c, errors.New("iam.token (SPX_INGEST_IAM_TOKEN) must contain 32–512 bytes")
	}

	c.Cache.URL = c.Cache.ResolveURL()

	return c, c.Validate()
}

// Validate 校验监听地址、依赖连接与代理网段。
func (c Config) Validate() error {
	if err := configuration.Validate(c); err != nil {
		return err
	}

	if err := c.Cache.Validate(); err != nil {
		return err
	}
	if c.HTTP.MaxBodyBytes < 0 || c.HTTP.RatePerSec < 0 || c.HTTP.RateBurst < 0 {
		return errors.New("http_channel limits must not be negative")
	}
	return c.Logging.Validate()
}

// ValidateServe 在创建运行资源前校验服务专用前置条件：
// Kafka 是唯一缓冲层，没有 broker 地址服务无法履行"不丢数据"承诺，启动即失败。
// IAM 凭证在管理 API（第五组）落地时才被消费，届时随路由一并启用必填校验；
// 现在只做格式校验：填了就必须是 32–512 字节，避免部署期埋下无效凭证。
func (c Config) ValidateServe() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if len(c.Kafka.Brokers) == 0 {
		return errors.New("kafka.brokers (SPX_KAFKA_BROKERS) is required to serve")
	}
	return nil
}
