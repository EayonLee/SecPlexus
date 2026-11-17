package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// Security 描述 Kafka 链路的传输与身份安全配置。
//
// 与 Confluent / Kafka 官方客户端的 security.protocol + sasl.mechanism 对应：
// 取值是闭集，未知取值在加载期拒绝，不做"猜一个最接近的"映射。
type Security struct {
	// Protocol 是 plaintext / ssl / sasl_plaintext / sasl_ssl。
	Protocol string
	// Mechanism 是 PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512；仅 SASL 协议下使用。
	Mechanism string
	Username  string
	// PasswordEnv 是存放密码的环境变量名：配置里不出现明文口令。
	PasswordEnv string
	// CAFile 是自定义 CA 文件；空表示使用系统根证书。
	CAFile string
}

// 支持的协议与机制。与 Kafka 官方配置项取值一致，便于运维照抄。
var (
	protocols  = map[string]bool{"plaintext": true, "ssl": true, "sasl_plaintext": true, "sasl_ssl": true}
	mechanisms = map[string]bool{"PLAIN": true, "SCRAM-SHA-256": true, "SCRAM-SHA-512": true}
)

// Options 把安全配置翻译成 franz-go 客户端选项。
//
// 未配置协议时返回空选项：保持既有明文行为，升级不改变默认。
func Options(security Security) ([]kgo.Opt, error) {
	protocol := security.Protocol
	if protocol == "" {
		protocol = "plaintext"
	}

	if !protocols[protocol] {
		return nil, fmt.Errorf("unsupported kafka security protocol %q", security.Protocol)
	}

	options := make([]kgo.Opt, 0, 2)

	if protocol == "ssl" || protocol == "sasl_ssl" {
		tlsConfig, err := tlsConfiguration(security.CAFile)
		if err != nil {
			return nil, err
		}

		options = append(options, kgo.DialTLSConfig(tlsConfig))
	}

	if protocol == "sasl_plaintext" || protocol == "sasl_ssl" {
		mechanism, err := saslMechanism(security)
		if err != nil {
			return nil, err
		}

		options = append(options, kgo.SASL(mechanism))
	}

	return options, nil
}

// tlsConfiguration 构建客户端 TLS 配置；自定义 CA 与系统根证书二者择一。
func tlsConfiguration(caFile string) (*tls.Config, error) {
	configuration := &tls.Config{MinVersion: tls.VersionTLS12}

	if caFile == "" {
		return configuration, nil
	}

	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("could not read kafka CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("kafka CA file contains no usable certificate")
	}

	configuration.RootCAs = pool

	return configuration, nil
}

// saslMechanism 按机制名构造认证器；口令从环境变量读取。
func saslMechanism(security Security) (sasl.Mechanism, error) {
	if security.Username == "" {
		return nil, errors.New("kafka sasl requires a username")
	}

	// 机制名先按闭集校验，再构造认证器：未知取值不应走到凭据读取。
	if !mechanisms[security.Mechanism] {
		return nil, fmt.Errorf("unsupported kafka sasl mechanism %q", security.Mechanism)
	}

	password, err := password(security.PasswordEnv)
	if err != nil {
		return nil, err
	}

	switch security.Mechanism {
	case "PLAIN":
		return plain.Auth{User: security.Username, Pass: password}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: security.Username, Pass: password}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: security.Username, Pass: password}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("unsupported kafka sasl mechanism %q", security.Mechanism)
	}
}

// password 从环境变量读取口令；缺失即失败，不用空口令尝试连接。
func password(env string) (string, error) {
	if env == "" {
		return "", errors.New("kafka sasl requires a password environment variable name")
	}

	value, ok := os.LookupEnv(env)
	if !ok || value == "" {
		return "", fmt.Errorf("environment variable %s is empty", env)
	}

	return value, nil
}
