// Package config 只负责 IAM 配置加载和校验，不建立数据库或缓存连接。
package config

import (
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// App 描述 IAM 的公开服务和反向代理配置。
type App struct {
	Name              string   `yaml:"name"`                                     // 服务名称。
	Host              string   `yaml:"host" validate:"required,ip|eq=localhost"` // 公开接口监听地址。
	Port              int      `yaml:"port" validate:"gte=1,lte=65535"`          // 公开接口监听端口。
	PublicURL         string   `yaml:"public_url"`                               // 浏览器访问的公开地址，用于 Cookie 和 CSRF 校验。
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs" validate:"dive,cidr"` // 允许转发客户端地址的代理网段。
}

// Addr 返回公开 HTTP 服务的监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Database 描述 PostgreSQL 连接；URL 非空时优先使用 URL。
type Database struct {
	Host     string `yaml:"host" validate:"required"`        // 监听或连接地址。
	Port     int    `yaml:"port" validate:"gte=1,lte=65535"` // 监听或连接端口。
	Name     string `yaml:"name"`                            // 数据库名称。
	User     string `yaml:"user"`                            // 数据库用户名。
	Password string `yaml:"password"`                        // 数据库连接密码。
	SSLMode  string `yaml:"sslmode"`                         // PostgreSQL TLS 模式。
	URL      string `yaml:"url"`                             // 可选的完整连接地址。
}

// connection 依据配置字段构造连接地址，不建立网络连接。
func (d Database) connection(user, password string) string {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Path:   "/" + d.Name,
		User:   url.UserPassword(user, password),
	}
	q := url.Values{"sslmode": {d.SSLMode}}
	u.RawQuery = q.Encode()
	return u.String()
}

// Cache 描述 Redis 连接及 ACL 用户；URL 非空时优先使用 URL。
type Cache struct {
	Host     string `yaml:"host" validate:"required"`        // 监听或连接地址。
	Port     int    `yaml:"port" validate:"gte=1,lte=65535"` // 监听或连接端口。
	User     string `yaml:"user"`                            // Redis ACL 用户名；默认使用 default。
	Password string `yaml:"password"`                        // Redis 连接密码。
	DB       int    `yaml:"db"`                              // Redis 逻辑数据库编号。
	URL      string `yaml:"url"`                             // 可选的完整连接地址。
}

// InternalClient 描述允许调用内部接口的服务及其权限。
type InternalClient struct {
	Token   string   `yaml:"token"`   // 内部调用凭证。
	Actions []string `yaml:"actions"` // 允许调用的权限列表。
}

// Internal 描述服务间通信使用的监听、TLS 和客户端配置。
type Internal struct {
	Host        string                    `yaml:"host" validate:"required,ip|eq=localhost"` // 监听或连接地址。
	Port        int                       `yaml:"port" validate:"gte=1,lte=65535"`          // 监听或连接端口。
	TLSCertFile string                    `yaml:"tls_cert_file"`                            // 服务端 TLS 证书路径。
	TLSKeyFile  string                    `yaml:"tls_key_file"`                             // 服务端 TLS 私钥路径。
	Clients     map[string]InternalClient `yaml:"clients"`                                  // 允许访问内部接口的服务列表。
}

// Addr 返回内部 HTTP 服务的监听地址。
func (i Internal) Addr() string { return net.JoinHostPort(i.Host, strconv.Itoa(i.Port)) }

// Auth 描述会话、二次认证和登录限制。
type Auth struct {
	CSRFKey                string        `yaml:"csrf_key"`                 // CSRF 令牌签名密钥。
	SessionIdleTimeout     time.Duration `yaml:"session_idle_timeout"`     // 会话空闲超时时间。
	SessionAbsoluteTimeout time.Duration `yaml:"session_absolute_timeout"` // 会话最长有效时间。
	ReauthWindow           time.Duration `yaml:"reauth_window"`            // 二次认证有效时间。
	LoginMaxFailures       int           `yaml:"login_max_failures"`       // 允许的连续登录失败次数。
	LoginLockDuration      time.Duration `yaml:"login_lock_duration"`      // 登录失败后的锁定时间。
}

// Config 包含 IAM 启动、基础组件和身份管理所需的配置。
type Config struct {
	App       App      `yaml:"app"`      // 公开服务配置。
	DB        Database `yaml:"db"`       // PostgreSQL 连接配置。
	Cache     Cache    `yaml:"cache"`    // Redis 连接配置。
	Internal  Internal `yaml:"internal"` // 内部服务配置。
	Auth      Auth     `yaml:"auth"`     // 认证策略配置。
	Bootstrap struct {
		Username string `yaml:"username"` // 初始化管理员用户名。
		Password string `yaml:"password"` // 初始管理员密码。
	} `yaml:"bootstrap"` // 首次初始化管理员配置。
	Retention struct {
		AuditDays   int `yaml:"audit_days"`   // 审计记录保留天数。
		SessionDays int `yaml:"session_days"` // 已结束会话保留天数。
	} `yaml:"retention"` // 审计和会话保留期限。
	Logging logging.Config `yaml:"logging"` // 日志输出与轮转配置。
}

// Load 加载配置，补全连接地址并校验配置值。
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

	if c.DB.URL == "" {
		c.DB.URL = c.DB.connection(c.DB.User, c.DB.Password)
	}

	if c.Cache.URL == "" {
		u := url.URL{
			Scheme: "redis",
			Host:   net.JoinHostPort(c.Cache.Host, strconv.Itoa(c.Cache.Port)),
			Path:   "/" + strconv.Itoa(c.Cache.DB),
		}
		if c.Cache.User != "" || c.Cache.Password != "" {
			u.User = url.UserPassword(c.Cache.User, c.Cache.Password)
		}

		c.Cache.URL = u.String()
	}

	return c, c.Validate()
}

// Validate 校验监听地址、依赖连接和认证策略。
func (c Config) Validate() error {
	if err := configuration.Validate(c); err != nil {
		return err
	}

	if c.App.Name == "" || len(c.App.Name) > 64 {
		return errors.New("app.name is required and limited to 64 characters")
	}

	origin, err := url.Parse(c.App.PublicURL)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return errors.New("app.public_url must be an HTTP(S) origin without credentials")
	}

	a := c.Auth
	if a.SessionIdleTimeout <= 0 || a.SessionAbsoluteTimeout < a.SessionIdleTimeout || a.SessionAbsoluteTimeout > 7*24*time.Hour || a.ReauthWindow <= 0 || a.LoginMaxFailures < 1 || a.LoginMaxFailures > 100 || a.LoginLockDuration <= 0 || c.Retention.AuditDays < 1 || c.Retention.AuditDays > 36500 || c.Retention.SessionDays < 1 || c.Retention.SessionDays > 36500 {
		return errors.New("invalid IAM timeouts or limits")
	}

	if c.DB.Name == "" || c.DB.User == "" || c.Cache.DB < 0 {
		return errors.New("invalid database or Redis configuration")
	}

	if _, err := redis.ParseURL(c.Cache.URL); err != nil {
		return errors.New("invalid cache connection configuration")
	}

	if _, err := pgx.ParseConfig(c.DB.URL); err != nil {
		return errors.New("invalid database connection configuration")
	}
	if err := c.ValidateSecurity(false); err != nil {
		return err
	}
	return c.Logging.Validate()
}

// ValidateSecurity 校验已提供的安全值；required 仅在运行认证服务时要求完整配置。
func (c Config) ValidateSecurity(required bool) error {
	if (required || c.Auth.CSRFKey != "") && len(c.Auth.CSRFKey) != 32 {
		return errors.New("CSRF signing key must contain exactly 32 bytes")
	}
	if required && len(c.Internal.Clients) == 0 {
		return errors.New("at least one internal client is required")
	}
	catalog := authorization.Catalog()
	for id, client := range c.Internal.Clients {
		if id == "" || len(client.Actions) == 0 || ((required || client.Token != "") && (len(client.Token) < 32 || len(client.Token) > 512)) {
			return errors.New("internal clients require an ID, 32–512 byte token and allowed actions")
		}
		for _, action := range client.Actions {
			if !slices.ContainsFunc(catalog, func(permission authorization.Permission) bool { return permission.Key == action }) {
				return errors.New("internal client action is not registered in the contract")
			}
		}
	}
	return nil
}

// ValidateServe 在创建运行资源前校验服务专用前置条件。
func (c Config) ValidateServe() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := c.ValidateSecurity(true); err != nil {
		return err
	}
	if c.Internal.TLSCertFile == "" || c.Internal.TLSKeyFile == "" {
		return errors.New("internal TLS certificate/key are required")
	}
	return nil
}
