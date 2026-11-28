// Package config 只负责 Ingest 配置加载和校验，不创建运行资源。
package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/clickhouse"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/postgresql"
	redisconn "github.com/EayonLee/SecPlexus/src/packages/go/redis"
)

// App 描述 Ingest 的统一监听配置。独立部署的原生 TLS 由 HTTP 接入配置提供。
type App struct {
	Host string `yaml:"host" validate:"required,ip|eq=localhost"` // HTTP 监听地址。
	Port int    `yaml:"port" validate:"gte=1,lte=65535"`          // HTTP 监听端口。
}

// Addr 返回 HTTP 服务的监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Kafka 是平台 Kafka 集群的连接配置。主题和角色凭据由 Events 定义。
type Kafka struct {
	Brokers       []string `yaml:"brokers" validate:"dive,hostname_port"`
	Protocol      string   `yaml:"security_protocol"`
	CAFile        string   `yaml:"tls_ca_file"`
	Namespace     string   `yaml:"namespace"`
	RetentionDays int64    `yaml:"retention_days"`
}

// RetentionMilliseconds 返回部署要求的主题保留时间。
func (k Kafka) RetentionMilliseconds() (int64, error) {
	const day = int64(24 * time.Hour / time.Millisecond)
	if k.RetentionDays <= 0 || k.RetentionDays > math.MaxInt64/day {
		return 0, errors.New("kafka.retention_days (SPX_KAFKA_RETENTION_DAYS) must be a positive number of days within the Kafka range")
	}
	return k.RetentionDays * day, nil
}

// KafkaIdentity 是平台 Kafka 角色的认证凭据。
type KafkaIdentity struct {
	Mechanism string `yaml:"sasl_mechanism"`
	Username  string `yaml:"sasl_username"`
	Password  string `yaml:"sasl_password"`
}

// Security 将共用传输设置与该角色的身份组合成客户端配置。
func (k Kafka) Security(identity KafkaIdentity) kafka.Security {
	return kafka.Security{
		Protocol: k.Protocol, CAFile: k.CAFile,
		Mechanism: identity.Mechanism, Username: identity.Username, Password: identity.Password,
	}
}

// Pipeline 描述解析流水线的容量参数。
type Pipeline struct {
	MaxInFlightBytes int64         `yaml:"max_in_flight_bytes" validate:"gt=0"` // 解析队列和 worker 的输入字节预算。
	Workers          int           `yaml:"workers"`                             // 解析 worker 数；0 表示按 CPU 核数。
	QueueDepth       int           `yaml:"queue_depth"`                         // 待解析队列长度；0 表示 10000，队列满时阻塞接收。
	BatchSize        int           `yaml:"batch_size"`                          // 每批事件数；0 表示 500。
	FlushInterval    time.Duration `yaml:"flush_interval"`                      // 批量最长等待；0 表示 2 秒。
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
	MaxBodyBytes          int64   `yaml:"max_body_bytes"`                          // 请求体上限，超出返回 413；0 表示 10 MiB。
	MaxConcurrentRequests int     `yaml:"max_concurrent_requests" validate:"gt=0"` // 设备推送处理并发上限。
	RatePerSec            float64 `yaml:"rate_per_sec"`                            // 每接入点每秒请求配额；0 表示 50。
	RateBurst             int     `yaml:"rate_burst"`                              // 每接入点突发额度；0 表示 200。
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

// InputDomain 是设备与外部数据源的接入配置。
type InputDomain struct {
	HTTP   HTTPChannel  `yaml:"http"`
	Syslog Syslog       `yaml:"syslog"`
	Kafka  KafkaChannel `yaml:"kafka"`
}

// KafkaChannel 是外部 Kafka 接入的刷新设置与传输身份。
type KafkaChannel struct {
	ReloadInterval time.Duration                     `yaml:"reload_interval"`
	Credentials    map[string]kafka.SourceCredential `yaml:"credentials"`
}

// ClickHouse 描述事件存储的连接和数据库角色。
type ClickHouse struct {
	URL               string        `yaml:"url"`
	Database          string        `yaml:"database"`
	Username          string        `yaml:"username"`
	Password          string        `yaml:"password"`
	ReaderUsername    string        `yaml:"reader_username"`
	ReaderPassword    string        `yaml:"reader_password"`
	MigrationUsername string        `yaml:"migration_username"`
	MigrationPassword string        `yaml:"migration_password"`
	Timeout           time.Duration `yaml:"timeout"`
}

// Events 描述内部事件主题与 ClickHouse 写入消费组。
type Events struct {
	Topic         string        `yaml:"-"`
	ConsumerGroup string        `yaml:"-"`
	FailureTopic  string        `yaml:"-"`
	Publisher     KafkaIdentity `yaml:"publisher"`
	Consumer      KafkaIdentity `yaml:"consumer"`
	BatchRecords  int           `yaml:"batch_records"`
	BatchBytes    int           `yaml:"batch_bytes"`
	CheckInterval time.Duration `yaml:"check_interval"`
}

// Config 包含 Ingest 各运行角色的配置。
type Config struct {
	PostgreSQL postgresql.Config `yaml:"postgresql"`
	App        App               `yaml:"app"`
	Pipeline   Pipeline          `yaml:"pipeline"`
	Input      InputDomain       `yaml:"input"`
	Kafka      Kafka             `yaml:"kafka"`
	Events     Events            `yaml:"events"`
	ClickHouse ClickHouse        `yaml:"clickhouse"`
	Redis      redisconn.Config  `yaml:"redis"`
	IAM        IAM               `yaml:"iam"`
	Logging    logging.Config    `yaml:"logging"`
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

	c.IAM.BaseURL = (&url.URL{Scheme: "http", Host: net.JoinHostPort(c.IAM.Host, strconv.Itoa(c.IAM.Port))}).String()
	c.Redis.URL = c.Redis.ResolveURL()
	c.Events.Topic = c.Kafka.Namespace + ".ingest.events"
	c.Events.FailureTopic = c.Kafka.Namespace + ".ingest.event-failures"
	c.Events.ConsumerGroup = c.Kafka.Namespace + "-ingest-events"
	if c.Input.Syslog.Host == "" {
		c.Input.Syslog.Host = c.App.Host
	}
	return c, nil
}

var kafkaNamespace = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ValidateKafka 校验平台 Kafka 连接、资源命名、保留时间和调用身份。
func (c Config) ValidateKafka(identity KafkaIdentity) error {
	if len(c.Kafka.Brokers) == 0 {
		return errors.New("kafka.brokers (SPX_KAFKA_BROKERS) is required")
	}
	if _, err := c.Kafka.RetentionMilliseconds(); err != nil {
		return err
	}
	if !kafkaNamespace.MatchString(c.Kafka.Namespace) {
		return errors.New("kafka.namespace must start with a lowercase letter and contain at most 63 lowercase letters, digits or hyphens")
	}
	_, err := kafka.Options(c.Kafka.Security(identity))
	return err
}

// ValidateServe 校验服务必需配置并读取 TLS 密钥对，不连接依赖。
func (c Config) ValidateServe() (*tls.Config, error) {
	if err := configuration.Validate(c); err != nil {
		return nil, err
	}
	if err := c.PostgreSQL.Validate(); err != nil {
		return nil, err
	}
	if err := c.Redis.Validate(); err != nil {
		return nil, err
	}
	if err := c.Logging.Validate(); err != nil {
		return nil, err
	}
	if c.IAM.Host == "0.0.0.0" || c.IAM.Host == "::" {
		return nil, errors.New("iam.host (SPX_IAM_HOST) cannot be a wildcard")
	}
	if err := iamclient.ValidateConfig(c.IAM.BaseURL, c.IAM.Token, true); err != nil {
		return nil, errors.New("iam.token (SPX_INGEST_IAM_TOKEN) must contain 32–512 bytes")
	}
	for ref, credential := range c.Input.Kafka.Credentials {
		if !input.ValidKafkaCredential(ref) {
			return nil, errors.New("input.kafka.credentials names must start with a lowercase letter and contain at most 64 lowercase letters, digits, hyphens or underscores")
		}
		if err := configuration.Validate(credential); err != nil {
			return nil, fmt.Errorf("input.kafka.credentials.%s requires up to 16 valid brokers", ref)
		}
		if _, err := kafka.Options(credential.Security); err != nil {
			return nil, fmt.Errorf("input.kafka.credentials.%s: %w", ref, err)
		}
	}
	if c.ClickHouse.URL != "" {
		if err := c.ClickHouse.Validate(); err != nil {
			return nil, err
		}
		if c.ClickHouse.ReaderUsername == "" {
			return nil, errors.New("clickhouse.reader_username (SPX_INGEST_CLICKHOUSE_READER_USERNAME) is required")
		}
	}
	if err := c.ValidateKafka(c.Events.Publisher); err != nil {
		return nil, err
	}
	http := c.Input.HTTP
	if http.MaxBodyBytes < 0 || http.RatePerSec < 0 || http.RateBurst < 0 {
		return nil, errors.New("http limits must not be negative")
	}
	if (http.TLSCertFile == "") != (http.TLSKeyFile == "") {
		return nil, errors.New("input.http.tls_cert_file and input.http.tls_key_file must be set together")
	}
	if http.TLSCertFile == "" {
		return nil, nil
	}
	certificate, err := tls.LoadX509KeyPair(http.TLSCertFile, http.TLSKeyFile)
	if err != nil {
		return nil, errors.New("input.http.tls_cert_file and input.http.tls_key_file must contain a readable matching TLS key pair")
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, nil
}

// Validate 校验 ClickHouse 连接和事件写入时限。
func (a ClickHouse) Validate() error {
	if !clickhouse.DatabaseNameValid(a.Database) {
		return errors.New("clickhouse.database must be a SQL identifier")
	}
	if _, err := clickhouse.ParseEndpoint(a.URL); err != nil {
		return fmt.Errorf("clickhouse.url (SPX_CLICKHOUSE_URL): %w", err)
	}
	if a.Timeout <= 0 || a.Timeout > time.Minute {
		return errors.New("clickhouse.timeout must be positive and at most one minute")
	}
	return nil
}

// ValidateStoreEvents 只校验事件写入进程使用的配置。
func (c Config) ValidateStoreEvents() error {
	if err := c.ClickHouse.Validate(); err != nil {
		return err
	}
	if err := c.ValidateKafka(c.Events.Consumer); err != nil {
		return err
	}
	if err := c.Logging.Validate(); err != nil {
		return err
	}
	e := c.Events
	if e.ConsumerGroup == "" || e.FailureTopic == "" || e.FailureTopic == e.Topic {
		return errors.New("events requires a consumer group and a distinct failure topic")
	}
	if e.BatchRecords <= 0 || e.BatchBytes <= 0 || e.BatchBytes > 32<<20 || e.CheckInterval <= 0 {
		return errors.New("events batch size and check interval must be positive; batch_bytes is limited to 32 MiB")
	}
	return nil
}
