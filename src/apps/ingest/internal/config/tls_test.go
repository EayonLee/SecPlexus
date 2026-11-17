package config_test

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/config"
)

// 回归：只配一半 TLS 参数必须拒绝启动，否则会以为链路已加密。
func TestValidateServeRejectsHalfTLS(t *testing.T) {
	base := config.Config{}

	withCert := base
	withCert.Input.HTTP.TLSCertFile = "/tmp/cert.pem"
	if err := withCert.ValidateServe(); err == nil {
		t.Fatal("只有证书没有私钥时没有报错")
	}

	withKey := base
	withKey.Input.HTTP.TLSKeyFile = "/tmp/key.pem"
	if err := withKey.ValidateServe(); err == nil {
		t.Fatal("只有私钥没有证书时没有报错")
	}
}
