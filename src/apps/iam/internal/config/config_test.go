package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "config.yaml")
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}

	return file
}

func TestConfigurationPrecedence(t *testing.T) {
	file := configFile(t)
	data, _ := os.ReadFile(file)
	data = []byte(strings.Replace(string(data), "port: 8081", "port: 9081", 1))
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPX_IAM_PORT", "9082")
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.App.Port != 9082 || cfg.Auth.SessionIdleTimeout != 30*time.Minute || cfg.Auth.SessionAbsoluteTimeout != 8*time.Hour {
		t.Fatal("incorrect configuration precedence")
	}

	if cfg.Bootstrap.Username != "admin" || cfg.Bootstrap.Password != "secplexus" || cfg.Cache.Password != "secplexus" || cfg.Cache.User != "default" {
		t.Fatal("incorrect authentication defaults")
	}

	if cfg.DB.URL != "postgres://secplexus:secplexus@127.0.0.1:5432/secplexus?sslmode=disable" {
		t.Fatal("incorrect local database defaults")
	}
}

func TestConfigurationRejectsUnknownFields(t *testing.T) {
	file := configFile(t)
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	if _, err := f.WriteString("\ntypo: true\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestConfigurationRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"0", "65536", "secret-invalid"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("SPX_IAM_PORT", port)
			if _, err := Load(configFile(t)); err == nil {
				t.Fatal("invalid port accepted")
			}
		})
	}
}

func TestPasswordsRemainLiteralInConnectionURLs(t *testing.T) {
	password := "space @:/?#${VALUE}"
	t.Setenv("SPX_POSTGRES_PASSWORD", password)
	c, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(c.DB.URL)
	if err != nil {
		t.Fatal("invalid database URL")
	}

	actual, _ := parsed.User.Password()
	if actual != password || strings.Contains(c.DB.URL, password) {
		t.Fatal("password must round-trip without interpolation or URL corruption")
	}
}

func TestSharedDatabaseDefaultsAndExplicitModuleOverride(t *testing.T) {
	file := configFile(t)
	t.Setenv("SPX_POSTGRES_HOST", "shared-database")
	t.Setenv("SPX_POSTGRES_DATABASE", "platform")
	t.Setenv("SPX_POSTGRES_USER", "platform_user")
	t.Setenv("SPX_POSTGRES_PASSWORD", "platform password")
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DB.Host != "shared-database" || cfg.DB.Name != "platform" || cfg.DB.User != "platform_user" {
		t.Fatal("module did not inherit global PostgreSQL settings")
	}

	data, _ := os.ReadFile(file)
	replaced := strings.NewReplacer("host: \"127.0.0.1\"", "host: \"module-database\"").Replace(string(data))
	t.Setenv("SPX_IAM_POSTGRES_HOST", "module-database")
	t.Setenv("SPX_IAM_POSTGRES_USER", "module_user")
	if err = os.WriteFile(file, []byte(replaced), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err = Load(file)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DB.Host != "module-database" || cfg.DB.User != "module_user" || cfg.DB.Name != "platform" {
		t.Fatal("module override was overwritten by global settings")
	}
}

func TestGlobalRedisConnection(t *testing.T) {
	t.Setenv("SPX_REDIS_HOST", "shared-cache")
	t.Setenv("SPX_REDIS_PORT", "6380")
	t.Setenv("SPX_REDIS_DB", "7")
	t.Setenv("SPX_REDIS_PASSWORD", "cache @?# password")
	t.Setenv("SPX_REDIS_USER", "analyst:@/cache")
	cfg, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(cfg.Cache.URL)
	if err != nil {
		t.Fatal(err)
	}

	password, _ := parsed.User.Password()
	if parsed.Host != "shared-cache:6380" || parsed.Path != "/7" || password != "cache @?# password" || parsed.User.Username() != "analyst:@/cache" {
		t.Fatal("invalid shared Redis configuration")
	}
}

func TestPostgresModelessEnvironment(t *testing.T) {
	t.Setenv("SPX_POSTGRES_MODELESS", "require")
	cfg, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(cfg.DB.URL)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DB.SSLMode != "require" || parsed.Query().Get("sslmode") != "require" {
		t.Fatal("数据库 TLS 模式未从 SPX_POSTGRES_MODELESS 传递到连接地址")
	}
}

// TestRedisURLOverride 保留完整连接地址的优先级，不混入其他连接字段。
func TestRedisURLOverride(t *testing.T) {
	t.Setenv("SPX_REDIS_USER", "ignored-user")
	t.Setenv("SPX_REDIS_PASSWORD", "ignored-password")
	t.Setenv("SPX_REDIS_URL", "redis://acl-user:encoded%40password@cache.example:6380/3")
	cfg, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Cache.URL != "redis://acl-user:encoded%40password@cache.example:6380/3" {
		t.Fatal("完整 Redis URL 不应被结构化连接字段覆盖")
	}
}

// TestInvalidRedisURLRejectedAtLoading 让连接格式错误在配置加载时明确失败。
func TestInvalidRedisURLRejectedAtLoading(t *testing.T) {
	for _, value := range []string{"http://cache", "redis://cache/not-a-database", "redis://cache:wrong"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SPX_REDIS_URL", value)
			if _, err := Load(configFile(t)); err == nil {
				t.Fatal("错误连接格式未在配置加载时拒绝")
			}
		})
	}
}
