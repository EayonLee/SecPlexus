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
	// EventTopic 是归一化成功事件的发布 topic。
	EventTopic string `yaml:"event_topic" validate:"required"`
	// ParseFailureTopic 是解析失败事件的发布 topic。
	// 与事件 topic 分开：解析失败的数据不是 OCSF，混发会污染下游消费方。
	ParseFailureTopic string `yaml:"parse_failure_topic" validate:"required"`
	// 传输与身份安全；与 Kafka 官方 security.protocol / sasl.mechanism 取值一致。
	SecurityProtocol string `yaml:"security_protocol"`
	SASLMechanism    string `yaml:"sasl_mechanism"`
	SASLUsername     string `yaml:"sasl_username"`
	// SASLPasswordEnv 是存放口令的环境变量名：配置里不出现明文口令。
	SASLPasswordEnv string `yaml:"sasl_password_env"`
	TLSCAFile       string `yaml:"tls_ca_file"` // 唯一发布 topic；解析失败事件也发这里（parse.outcome 区分）。
}

// Database 描述 PostgreSQL 连接；接入点与解析规则存在这里。
type Database struct {
	Name     string `yaml:"name"`     // 数据库名。
	Host     string `yaml:"host"`     // 连接地址。
	Port     int    `yaml:"port"`     // 连接端口。
	User     string `yaml:"user"`     // 账号。
	Password string `yaml:"password"` // 密码；生产必须由环境变量覆盖。
	SSLMode  string `yaml:"sslmode"`  // TLS 模式。
}

// DSN 由各字段拼出连接地址，不建立连接。
func (d Database) DSN() string {
	query := url.Values{}
	if d.SSLMode != "" {
		query.Set("sslmode", d.SSLMode)
	}

	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.User, d.Password),
		Host:     net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Path:     "/" + d.Name,
		RawQuery: query.Encode(),
	}).String()
}

// Pipeline 描述解析流水线的容量参数；零值由管道填默认值，配置里只写想改的。',
type Pipeline struct {
	Workers       int           `yaml:"workers"`        // 解析 worker 数；0 表示按 CPU 核数。
	QueueDepth    int           `yaml:"queue_depth"`    // 待解析队列长度；0 表示 10000，队列满时阻塞接收。
	BatchSize     int           `yaml:"batch_size"`     // 每批事件数；0 表示 500。
	FlushInterval time.Duration `yaml:"flush_interval"` // 批量最长等待；0 表示 2 秒。
}

// Syslog 描述 syslog 通道参数。
type Syslog struct {
	// BindHost 是 syslog 监听地址。默认跟随 app.host：
	// 前面有反向代理、只允许本机接收时，这个值必须与 app.host 一致，
	// 否则 syslog 端口会意外暴露到所有网卡。
	Host string `yaml:"host"` // syslog 监听地址；空表示跟随 app.host。
	// ReloadInterval 是端口绑定关系的刷新间隔；SIGHUP 可立即重载。
	ReloadInterval time.Duration `yaml:"reload_interval"`
}

// HTTPChannel 描述设备推送通道（HTTP）的参数。
type HTTPChannel struct {
	MaxBodyBytes int64   `yaml:"max_body_bytes"` // 请求体上限，超出返回 413；0 表示 10 MiB。
	RatePerSec   float64 `yaml:"rate_per_sec"`   // 每接入点每秒请求配额；0 表示 50。
	RateBurst    int     `yaml:"rate_burst"`     // 每接入点突发额度；0 表示 200。
	// TLSCertFile/TLSKeyFile 同时为空或同时设置：半套配置拒绝启动，避免"以为开了 TLS"。
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
}

// IAM 描述授权上游；Ingest 的管理 API 每个受保护请求都向 IAM 换取授权结论。
type IAM struct {
	Host    string `yaml:"host" validate:"required,hostname|ip"` // IAM 连接目标，IP 或域名。
	Port    int    `yaml:"port" validate:"gte=1,lte=65535"`      // IAM 连接端口。
	BaseURL string `yaml:"-"`                                    // 由结构化连接字段派生，不接受另一份 URL 配置。
	Token   string `yaml:"token"`                                // 调用 IAM 的服务凭证。
}

// InputDomain 是入站通道集合：数据从哪里进来。
//
// 与 OutputDomain 分开命名：Kafka 既能当入站通道（消费客户总线）也能当出站目标
// （发布 OCSF 事件），平级同名会让运维无法判断改的是哪一侧。
type InputDomain struct {
	HTTP   HTTPChannel  `yaml:"http"`   // 设备推送（HTTP）。
	Syslog Syslog       `yaml:"syslog"` // syslog 收包。
	Kafka  KafkaChannel `yaml:"kafka"`  // 消费客户已有总线。
}

// KafkaChannel 是入站 Kafka 的通道级参数。
// broker 与 topic 属于每个接入点（存在数据库里），这里只放进程级行为。
type KafkaChannel struct {
	// ReloadInterval 是接入点集合的刷新间隔；0 表示 30 秒。
	ReloadInterval time.Duration `yaml:"reload_interval"`
}

// OutputDomain 是出站目标集合：归一化后的事件写到哪里。
type OutputDomain struct {
	Kafka Kafka `yaml:"kafka"`
}

// Config 包含 Ingest 启动和运行所需的配置。
type Config struct {
	DB       Database       `yaml:"db"`       // 接入点与解析规则的存储。
	App      App            `yaml:"app"`      // 服务监听配置。
	Pipeline Pipeline       `yaml:"pipeline"` // 解析流水线容量。
	Input    InputDomain    `yaml:"input"`    // 入站通道：数据从哪里进来。
	Output   OutputDomain   `yaml:"output"`   // 出站目标：归一化后的事件写到哪里。
	Cache    cache.Config   `yaml:"cache"`    // 令牌限流用的 Redis。
	IAM      IAM            `yaml:"iam"`      // 管理 API 的授权上游。
	Logging  logging.Config `yaml:"logging"`  // 日志输出与轮转配置。
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

	if c.Input.Syslog.Host == "" {
		// 默认跟随 app.host：syslog 端口不能比主监听更宽松。
		c.Input.Syslog.Host = c.App.Host
	}

	return c, c.Validate()
}

// Validate 校验监听地址、依赖连接与代理网段。
func (c Config) Validate() error {
	if err := configuration.Validate(c); err != nil {
		return err
	}

	if c.DB.Host == "" || c.DB.Port <= 0 || c.DB.Name == "" {
		return errors.New("db.host, db.port and db.name are required")
	}
	if err := c.Cache.Validate(); err != nil {
		return err
	}
	if c.Input.HTTP.MaxBodyBytes < 0 || c.Input.HTTP.RatePerSec < 0 || c.Input.HTTP.RateBurst < 0 {
		return errors.New("http limits must not be negative")
	}
	if c.Input.Syslog.Host == "" {
		c.Input.Syslog.Host = c.App.Host
	}
	return c.Logging.Validate()
}

// ValidateServe 在创建运行资源前校验服务专用前置条件：
// Kafka 是唯一缓冲层，没有 broker 地址服务无法履行"不丢数据"承诺，启动即失败。
// IAM 凭证在管理 API（第五组）落地时才被消费，届时随路由一并启用必填校验；
// 现在只做格式校验：填了就必须是 32–512 字节，避免部署期埋下无效凭证。
func (c Config) ValidateServe() error {
	// TLS 证书与私钥必须成对出现：只配一半会让人以为链路已加密。
	if c.Input.HTTP.TLSCertFile != "" && c.Input.HTTP.TLSKeyFile == "" {
		return errors.New("http.tls_cert_file and http.tls_key_file must be set together")
	}
	if c.Input.HTTP.TLSKeyFile != "" && c.Input.HTTP.TLSCertFile == "" {
		return errors.New("http.tls_cert_file and http.tls_key_file must be set together")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if len(c.Output.Kafka.Brokers) == 0 {
		return errors.New("kafka.brokers (SPX_INGEST_KAFKA_BROKERS) is required to serve")
	}
	return nil
}
