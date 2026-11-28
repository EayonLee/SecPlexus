package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"

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
	Protocol string `yaml:"security_protocol"`
	// Mechanism 是 PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512；仅 SASL 协议下使用。
	Mechanism string `yaml:"sasl_mechanism"`
	Username  string `yaml:"sasl_username"`
	// Password 是配置阶段已解析的口令。
	Password string `yaml:"sasl_password"`
	// CAFile 是自定义 CA 文件；空表示使用系统根证书。
	CAFile string `yaml:"tls_ca_file"`
}

// SourceCredential 绑定外部 Kafka 的连接凭据与允许配置的引导服务器。
type SourceCredential struct {
	Brokers  []string `yaml:"brokers" validate:"required,min=1,max=16,dive,hostname_port"`
	Security `yaml:",squash"`
}

// Allows 校验引用的凭据只交给部署配置登记的 Kafka 引导服务器。
func (c SourceCredential) Allows(brokers []string) bool {
	return len(brokers) > 0 && len(brokers) <= len(c.Brokers) && !slices.ContainsFunc(brokers, func(broker string) bool {
		return !slices.Contains(c.Brokers, broker)
	})
}

// 支持的协议与机制。与 Kafka 官方配置项取值一致，便于运维照抄。
var (
	protocols  = map[string]bool{"plaintext": true, "ssl": true, "sasl_plaintext": true, "sasl_ssl": true}
	mechanisms = map[string]bool{"PLAIN": true, "SCRAM-SHA-256": true, "SCRAM-SHA-512": true}
)

// Options 把安全配置翻译成 franz-go 客户端选项。
//
// 未配置协议时使用明文连接。
func Options(security Security) ([]kgo.Opt, error) {
	protocol := security.Protocol
	if protocol == "" {
		protocol = "plaintext"
	}

	if !protocols[protocol] {
		return nil, errors.New("unsupported kafka security protocol")
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

// saslMechanism 使用已加载的凭据构造认证器。
func saslMechanism(security Security) (sasl.Mechanism, error) {
	if security.Username == "" {
		return nil, errors.New("kafka sasl requires a username")
	}

	// 机制名先按闭集校验，再构造认证器：未知取值不应走到凭据读取。
	if !mechanisms[security.Mechanism] {
		return nil, errors.New("unsupported kafka sasl mechanism")
	}

	if security.Password == "" {
		return nil, errors.New("kafka sasl requires a password")
	}

	switch security.Mechanism {
	case "PLAIN":
		return plain.Auth{User: security.Username, Pass: security.Password}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: security.Username, Pass: security.Password}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: security.Username, Pass: security.Password}.AsSha512Mechanism(), nil
	default:
		return nil, errors.New("unsupported kafka sasl mechanism")
	}
}
