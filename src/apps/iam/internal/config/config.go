// Package config 只负责 IAM 配置加载和校验，不建立数据库或缓存连接。
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/packages/go/cache"
	"github.com/EayonLee/SecPlexus/src/packages/go/configuration"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/jackc/pgx/v5"
)

// App 描述 IAM 的统一监听和反向代理配置。
type App struct {
	Host              string   `yaml:"host" validate:"required,ip|eq=localhost"` // HTTP 接口监听地址。
	Port              int      `yaml:"port" validate:"gte=1,lte=65535"`          // HTTP 接口监听端口。
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs" validate:"dive,cidr"` // 允许转发客户端地址的代理网段。
}

// Addr 返回统一 HTTP 服务的监听地址。
func (a App) Addr() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// Database 描述 PostgreSQL 连接；URL 非空时优先使用 URL。
type Database struct {
	Name     string `yaml:"name"`                            // 数据库名称。
	Host     string `yaml:"host" validate:"required"`        // 监听或连接地址。
	Port     int    `yaml:"port" validate:"gte=1,lte=65535"` // 监听或连接端口。
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

// ServiceClient 描述允许请求鉴权的服务及其权限。
type ServiceClient struct {
	Token   string   `yaml:"token"`   // 服务调用凭证。
	Actions []string `yaml:"actions"` // 允许调用的权限列表。
}

// Auth 描述会话、二次认证和登录限制。
type Auth struct {
	WebOrigin              string        `yaml:"web_origin"`               // 浏览器可信来源；与后端监听位置不同。
	CSRFKey                string        `yaml:"csrf_key"`                 // CSRF 令牌签名密钥。
	SessionIdleTimeout     time.Duration `yaml:"session_idle_timeout"`     // 会话空闲超时时间。
	SessionAbsoluteTimeout time.Duration `yaml:"session_absolute_timeout"` // 会话最长有效时间。
	ReauthWindow           time.Duration `yaml:"reauth_window"`            // 二次认证有效时间。
	LoginMaxFailures       int           `yaml:"login_max_failures"`       // 允许的连续登录失败次数。
	LoginLockDuration      time.Duration `yaml:"login_lock_duration"`      // 登录失败后的失败锁定时间。
	LoginIPRate            int           `yaml:"login_ip_rate"`            // 每个来源地址每分钟允许的认证请求数。
	LoginAccountRate       int           `yaml:"login_account_rate"`       // 每个账号每 15 分钟允许的认证请求数。
}

// Bootstrap 描述配置托管的内置管理员：每次启动都幂等写入数据库。
type Bootstrap struct {
	Username string `yaml:"username"` // 内置管理员用户名；改名会收回旧账号的内置标记。
	Password string `yaml:"password"` // 内置管理员密码；只能由配置更新。
}

// Retention 描述按天计算的清理窗口。
type Retention struct {
	AuditDays   int `yaml:"audit_days"`   // 审计记录保留天数。
	SessionDays int `yaml:"session_days"` // 已结束会话保留天数。
}

// Config 包含 IAM 启动、基础组件和账号与授权所需的配置。
type Config struct {
	App            App                      `yaml:"app"`             // 公开服务配置。
	DB             Database                 `yaml:"db"`              // PostgreSQL 连接配置。
	Cache          cache.Config             `yaml:"cache"`           // Redis 连接配置。
	ServiceClients map[string]ServiceClient `yaml:"service_clients"` // 服务调用方及允许的操作。
	Auth           Auth                     `yaml:"auth"`            // 认证策略配置。
	Bootstrap      Bootstrap                `yaml:"bootstrap"`       // 配置托管管理员的权威来源。
	Retention      Retention                `yaml:"retention"`       // 审计和会话保留期限。
	Logging        logging.Config           `yaml:"logging"`         // 日志输出与轮转配置。
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
	if err := configuration.Decode(settings, &c); err != nil {
		return c, err
	}

	if c.DB.URL == "" {
		c.DB.URL = c.DB.connection(c.DB.User, c.DB.Password)
	}

	c.Cache.URL = c.Cache.ResolveURL()

	return c, c.Validate()
}

// Validate 校验监听地址、依赖连接和认证策略。
func (c Config) Validate() error {
	if err := configuration.Validate(c); err != nil {
		return err
	}

	if err := c.ValidateTrustedProxies(); err != nil {
		return err
	}

	origin, err := url.Parse(c.Auth.WebOrigin)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return errors.New("auth.web_origin must be an HTTP(S) origin without credentials")
	}

	a := c.Auth
	if a.SessionIdleTimeout <= 0 {
		return errors.New("auth.session_idle_timeout must be greater than 0")
	}
	if a.SessionAbsoluteTimeout < a.SessionIdleTimeout || a.SessionAbsoluteTimeout > 7*24*time.Hour {
		return errors.New("auth.session_absolute_timeout must be at least session_idle_timeout and at most 168h")
	}
	if a.ReauthWindow <= 0 {
		return errors.New("auth.reauth_window must be greater than 0")
	}
	if a.LoginMaxFailures < 1 || a.LoginMaxFailures > 100 {
		return errors.New("auth.login_max_failures must be between 1 and 100")
	}
	if a.LoginLockDuration <= 0 {
		return errors.New("auth.login_lock_duration must be greater than 0")
	}
	if a.LoginIPRate < 1 || a.LoginIPRate > 10000 {
		return errors.New("auth.login_ip_rate must be between 1 and 10000")
	}
	if a.LoginAccountRate < 1 || a.LoginAccountRate > 10000 {
		return errors.New("auth.login_account_rate must be between 1 and 10000")
	}
	if c.Retention.AuditDays < 1 || c.Retention.AuditDays > 36500 {
		return errors.New("retention.audit_days must be between 1 and 36500")
	}
	if c.Retention.SessionDays < 1 || c.Retention.SessionDays > 36500 {
		return errors.New("retention.session_days must be between 1 and 36500")
	}
	if c.DB.Name == "" {
		return errors.New("db.name must not be empty")
	}
	if c.DB.User == "" {
		return errors.New("db.user must not be empty")
	}
	if err := c.Cache.Validate(); err != nil {
		return err
	}

	if _, err := pgx.ParseConfig(c.DB.URL); err != nil {
		return errors.New("db.url must be a valid PostgreSQL connection string; check db fields")
	}
	if err := c.ValidateSecurity(false); err != nil {
		return err
	}
	return c.Logging.Validate()
}

// ValidateSecurity 校验已提供的安全值；required 仅在运行认证服务时要求完整配置。
// ValidateTrustedProxies 拒绝"信任任意来源"的配置。
// Gin 官方文档 Trusted proxies 指出：可信代理列表决定 X-Forwarded-For 里哪些段可信；
// 写成 0.0.0.0/0 或 ::/0 等于信任任意客户端伪造的来源地址，按来源分桶的登录限流随即失效。
// 配置错误在启动时就失败，不留到运行期才发现。
func (c Config) ValidateTrustedProxies() error {
	for _, cidr := range c.App.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("app.trusted_proxy_cidrs contains an invalid CIDR %q", cidr)
		}
		// 前缀长度 0 就是 "所有来源"：0.0.0.0/0 与 ::/0 都会走到这里。
		if ones, _ := network.Mask.Size(); ones == 0 {
			return fmt.Errorf("app.trusted_proxy_cidrs must not trust every source (%s); list the actual proxy addresses", cidr)
		}
	}
	return nil
}

func (c Config) ValidateSecurity(required bool) error {
	if (required || c.Auth.CSRFKey != "") && len(c.Auth.CSRFKey) != 32 {
		return errors.New("auth.csrf_key (SPX_IAM_CSRF_KEY) must contain exactly 32 bytes")
	}
	if required && len(c.ServiceClients) == 0 {
		return errors.New("service_clients must contain at least one client")
	}
	catalog := authorization.Catalog()
	for id, client := range c.ServiceClients {
		if id == "" || len(client.Actions) == 0 || ((required || client.Token != "") && (len(client.Token) < 32 || len(client.Token) > 512)) {
			return errors.New("service_clients requires a nonempty ID, a 32–512 byte token (SPX_CONSOLE_IAM_TOKEN for Console) and allowed actions")
		}
		for _, action := range client.Actions {
			if !slices.ContainsFunc(catalog, func(permission authorization.Permission) bool { return permission.Key == action }) {
				return errors.New("service_clients.actions must use registered IAM permission keys")
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
	if err := c.ValidateAdministrator(); err != nil {
		return err
	}
	if err := c.ValidateSecurity(true); err != nil {
		return err
	}
	return nil
}

// ValidateAdministrator 校验配置托管账号，错误不包含配置的凭据值。
func (c Config) ValidateAdministrator() error {
	if _, err := account.NormalizeUsername(c.Bootstrap.Username); err != nil {
		return errors.New("bootstrap.username must identify the configured administrator")
	}
	if err := account.ValidatePassword(c.Bootstrap.Password); err != nil {
		// 报出实际长度而非密码值：截断（例如 .env 中未加引号的 '#' 之后被当作
		// 注释丢弃）会因此立刻自解释，且长度不是机密信息。
		return fmt.Errorf(
			"bootstrap.password must contain a valid 12–128 character administrator password (got %d)",
			utf8.RuneCountInString(c.Bootstrap.Password),
		)
	}
	return nil
}
