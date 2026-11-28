package kafka_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
)

// 未配置协议时保持既有明文行为：升级不改变默认。
func TestOptionsDefaultsToPlaintext(t *testing.T) {
	options, err := kafka.Options(kafka.Security{})
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 0 {
		t.Fatalf("默认应不追加任何选项，实际 %d 个", len(options))
	}
}

// 未知协议在加载期拒绝，不做"猜一个最接近的"映射。
func TestOptionsRejectsUnknownProtocol(t *testing.T) {
	if _, err := kafka.Options(kafka.Security{Protocol: "mtls"}); err == nil {
		t.Fatal("未知协议被接受")
	}
}

// ssl 协议必须产生 TLS 选项。
func TestOptionsBuildsTLSForSSL(t *testing.T) {
	options, err := kafka.Options(kafka.Security{Protocol: "ssl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 {
		t.Fatalf("ssl 应产生 1 个选项，实际 %d 个", len(options))
	}
}

// SASL 缺少用户名或口令来源时必须在启动期失败。
func TestOptionsRequiresSASLCredentials(t *testing.T) {
	if _, err := kafka.Options(kafka.Security{Protocol: "sasl_ssl", Mechanism: "PLAIN"}); err == nil {
		t.Fatal("缺少用户名时没有报错")
	}

	if _, err := kafka.Options(kafka.Security{
		Protocol: "sasl_ssl", Mechanism: "PLAIN", Username: "svc",
	}); err == nil {
		t.Fatal("缺少口令环境变量名时没有报错")
	}

	if _, err := kafka.Options(kafka.Security{
		Protocol: "sasl_ssl", Mechanism: "PLAIN", Username: "svc", Password: "",
	}); err == nil {
		t.Fatal("口令环境变量为空时没有报错")
	}
}

// 三种 SASL 机制都应被接受。
func TestOptionsAcceptsSASLMechanisms(t *testing.T) {

	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		options, err := kafka.Options(kafka.Security{
			Protocol: "sasl_ssl", Mechanism: mechanism, Username: "svc", Password: "test-password",
		})
		if err != nil {
			t.Fatalf("%s 被拒绝: %v", mechanism, err)
		}
		if len(options) != 2 {
			t.Fatalf("%s 应产生 TLS + SASL 两个选项，实际 %d 个", mechanism, len(options))
		}
	}

	if _, err := kafka.Options(kafka.Security{
		Protocol: "sasl_ssl", Mechanism: "GSSAPI", Username: "svc", Password: "test-password",
	}); err == nil {
		t.Fatal("未知机制被接受")
	}
}

// 自定义 CA 必须是可用证书；垃圾文件要在启动期失败。
func TestOptionsRejectsBrokenCAFile(t *testing.T) {
	directory := t.TempDir()
	broken := filepath.Join(directory, "ca.pem")
	if err := os.WriteFile(broken, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := kafka.Options(kafka.Security{Protocol: "ssl", CAFile: broken}); err == nil {
		t.Fatal("不可用的 CA 文件被接受")
	}

	if _, err := kafka.Options(kafka.Security{Protocol: "ssl", CAFile: filepath.Join(directory, "missing.pem")}); err == nil {
		t.Fatal("缺失的 CA 文件被接受")
	}
}
