package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configFile 把模块 config.yaml 复制到临时目录，供用例改写后加载。
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

// serveEnv 提供运行服务所需的必填环境变量。
func serveEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SPX_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("SPX_INGEST_IAM_TOKEN", strings.Repeat("t", 32))
}

func TestIngestListenerIsIndependentOfItsConnectionTarget(t *testing.T) {
	t.Setenv("SPX_INGEST_HOST", "ingest.corp")
	t.Setenv("SPX_INGEST_BIND_HOST", "0.0.0.0")
	cfg, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Host != "0.0.0.0" {
		t.Fatal("connection target changed the listener")
	}
}

func TestPreflightRejectsDependenciesThatCannotBeInitialized(t *testing.T) {
	for _, scenario := range []struct {
		field string
		env   map[string]string
	}{
		{"postgresql.url", map[string]string{"SPX_POSTGRESQL_URL": "postgres://[invalid"}},
		{"iam.token", map[string]string{"SPX_INGEST_IAM_TOKEN": ""}},
		{"tls", map[string]string{"SPX_INGEST_TLS_CERT_FILE": "missing.crt", "SPX_INGEST_TLS_KEY_FILE": "missing.key"}},
	} {
		t.Run(scenario.field, func(t *testing.T) {
			serveEnv(t)
			for name, value := range scenario.env {
				t.Setenv(name, value)
			}
			cfg, err := Load(configFile(t))
			if err == nil {
				_, err = cfg.ValidateServe()
			}
			if err == nil || !strings.Contains(err.Error(), scenario.field) {
				t.Fatalf("expected a diagnostic for %s, got %v", scenario.field, err)
			}
		})
	}
}

func TestConfigurationPrecedence(t *testing.T) {
	file := configFile(t)
	t.Setenv("SPX_INGEST_PORT", "9082")
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.App.Port != 9082 {
		t.Fatal("incorrect configuration precedence")
	}

	// 通道与 topic 的默认值来自 YAML，不来自 Go 代码。
	if cfg.Events.Topic != "spx.ingest.events" {
		t.Fatal("incorrect kafka topic default")
	}
	if cfg.IAM.BaseURL != "http://127.0.0.1:8081" {
		t.Fatal("incorrect IAM base URL default")
	}
	if !strings.Contains(cfg.Redis.URL, "127.0.0.1:6379") {
		t.Fatal("incorrect Redis URL default")
	}
}

func TestConfigurationRejectsUnknownFields(t *testing.T) {
	file := configFile(t)
	data, _ := os.ReadFile(file)
	if err := os.WriteFile(file, append(data, []byte("\nunknown_field: 1\n")...), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(file)
	if err == nil || !strings.Contains(err.Error(), "unknown configuration fields") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}
}

func TestKafkaRetentionDaysRejectsZero(t *testing.T) {
	t.Setenv("SPX_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("SPX_INGEST_IAM_TOKEN", strings.Repeat("t", 32))
	t.Setenv("SPX_KAFKA_RETENTION_DAYS", "0")
	cfg, err := Load(configFile(t))
	if err == nil {
		_, err = cfg.ValidateServe()
	}
	if err == nil || !strings.Contains(err.Error(), "retention_days") {
		t.Fatalf("expected a Kafka retention diagnostic, got %v", err)
	}
}

func TestCheckConfigRequiresKafkaBrokers(t *testing.T) {
	file := configFile(t)
	t.Setenv("SPX_INGEST_IAM_TOKEN", strings.Repeat("t", 32))

	_, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.ValidateServe()
	if err == nil || !strings.Contains(err.Error(), "kafka.brokers") {
		t.Fatalf("expected kafka.brokers field error, got %v", err)
	}
}

// 管理 API 落地前 token 不强制，但填了就必须是合法格式——避免部署期埋下无效凭证。
func TestValidateServeRejectsMalformedIAMToken(t *testing.T) {
	file := configFile(t)
	t.Setenv("SPX_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("SPX_INGEST_IAM_TOKEN", "short")

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ValidateServe(); err == nil || !strings.Contains(err.Error(), "32–512 bytes") {
		t.Fatalf("malformed token accepted: %v", err)
	}
}

func TestValidateServeAcceptsCompleteConfig(t *testing.T) {
	file := configFile(t)
	serveEnv(t)

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cfg.ValidateServe(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationRejectsWildcardIAMHost(t *testing.T) {
	file := configFile(t)
	serveEnv(t)
	data, _ := os.ReadFile(file)
	data = []byte(strings.Replace(string(data), "SPX_IAM_HOST-127.0.0.1", "SPX_IAM_HOST-0.0.0.0", 1))
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.ValidateServe()
	if err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard rejection, got %v", err)
	}
}

// 这些用例断言 YAML 里的默认值，必须与调用者的 shell 环境隔离：
// 本地 source 过 .env 时 SPX_* 会覆盖默认值，断言就变成与环境相关的假失败。
func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SPX_") {
			_ = os.Unsetenv(key)
		}
	}

	os.Exit(m.Run())
}

func TestValidateServeRejectsInvalidKafkaSecurity(t *testing.T) {
	serveEnv(t)
	t.Setenv("SPX_KAFKA_SECURITY_PROTOCOL", "unsupported")
	cfg, err := Load(configFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ValidateServe(); err == nil || !strings.Contains(err.Error(), "unsupported kafka") {
		t.Fatalf("invalid Kafka security configuration: %v", err)
	}
}

func TestKafkaRoleCredentialsAreIndependent(t *testing.T) {
	serveEnv(t)
	t.Setenv("SPX_KAFKA_SECURITY_PROTOCOL", "sasl_ssl")
	for _, endpoint := range []struct{ prefix, username, password string }{
		{"SPX_INGEST_KAFKA_PUBLISH_", "publisher", "publish-test-password"},
		{"SPX_INGEST_KAFKA_STORE_", "consumer", "store-test-password"},
	} {
		t.Setenv(endpoint.prefix+"SASL_MECHANISM", "SCRAM-SHA-256")
		t.Setenv(endpoint.prefix+"SASL_USERNAME", endpoint.username)
		t.Setenv(endpoint.prefix+"SASL_PASSWORD", endpoint.password)
	}
	t.Setenv("SPX_INGEST_SOURCE_SITE_A_PASSWORD", "source-site-a-test-password")
	t.Setenv("SPX_INGEST_SOURCE_SITE_B_PASSWORD", "source-site-b-test-password")
	file := configFile(t)
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	configured := strings.Replace(string(content), "    credentials: {}", `    credentials:
      site-a:
        brokers: [broker-a:9093]
        security_protocol: sasl_ssl
        sasl_mechanism: SCRAM-SHA-256
        sasl_username: source-a
        sasl_password: "${SPX_INGEST_SOURCE_SITE_A_PASSWORD-}"
      site-b:
        brokers: [broker-b:9093]
        security_protocol: sasl_ssl
        sasl_mechanism: SCRAM-SHA-256
        sasl_username: source-b
        sasl_password: "${SPX_INGEST_SOURCE_SITE_B_PASSWORD-}"`, 1)
	if err := os.WriteFile(file, []byte(configured), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPX_INGEST_SOURCE_SITE_A_PASSWORD", "rotated-in-parent")
	a, b := cfg.Input.Kafka.Credentials["site-a"], cfg.Input.Kafka.Credentials["site-b"]
	if a.Password != "source-site-a-test-password" || b.Password != "source-site-b-test-password" ||
		cfg.Events.Publisher.Password != "publish-test-password" || cfg.Events.Consumer.Password != "store-test-password" ||
		!a.Allows([]string{"broker-a:9093"}) || a.Allows([]string{"broker-b:9093"}) {
		t.Fatal("Kafka configuration did not capture independent credentials")
	}
}

func TestKafkaRetentionDefaultAndOverride(t *testing.T) {
	file := configFile(t)
	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.Kafka.RetentionMilliseconds(); err != nil || got != 604800000 {
		t.Fatalf("default retention: %d, %v", got, err)
	}
	t.Setenv("SPX_KAFKA_RETENTION_DAYS", "3")
	cfg, err = Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.Kafka.RetentionMilliseconds(); err != nil || got != 259200000 {
		t.Fatalf("custom retention: %d, %v", got, err)
	}
}

func TestEventResourceNamesShareTheKafkaNamespace(t *testing.T) {
 t.Setenv("SPX_KAFKA_NAMESPACE", "customer-a")
 cfg, err := Load(configFile(t))
 if err != nil {
  t.Fatal(err)
 }
 if cfg.Events.Topic != "customer-a.ingest.events" || cfg.Events.FailureTopic != "customer-a.ingest.event-failures" || cfg.Events.ConsumerGroup != "customer-a-ingest-events" {
  t.Fatalf("resources did not use the same Kafka namespace: %s, %s, %s", cfg.Events.Topic, cfg.Events.FailureTopic, cfg.Events.ConsumerGroup)
 }
}
