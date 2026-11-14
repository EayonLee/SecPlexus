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
	if cfg.Kafka.Topic != "spx.ingest.event" {
		t.Fatal("incorrect kafka topic default")
	}
	if !cfg.Channels.HTTP || !cfg.Channels.Syslog || !cfg.Channels.Kafka {
		t.Fatal("incorrect channel defaults")
	}
	if cfg.IAM.BaseURL != "http://127.0.0.1:8081" {
		t.Fatal("incorrect IAM base URL default")
	}
	if !strings.Contains(cfg.Cache.URL, "127.0.0.1:6379") {
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
	err = cfg.ValidateServe()
	if err == nil || !strings.Contains(err.Error(), "kafka.brokers") {
		t.Fatalf("expected kafka.brokers field error, got %v", err)
	}
}

func TestCheckConfigRequiresIAMToken(t *testing.T) {
	file := configFile(t)
	t.Setenv("SPX_KAFKA_BROKERS", "127.0.0.1:9092")

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.ValidateServe()
	if err == nil || !strings.Contains(err.Error(), "SPX_INGEST_IAM_TOKEN") {
		t.Fatalf("expected iam.token field error, got %v", err)
	}
}

func TestValidateServeAcceptsCompleteConfig(t *testing.T) {
	file := configFile(t)
	serveEnv(t)

	cfg, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.ValidateServe(); err != nil {
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

	_, err := Load(file)
	if err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard rejection, got %v", err)
	}
}
