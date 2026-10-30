package config

import "testing"

func TestProvidedIAMConfigurationIsValidatedDuringLoad(t *testing.T) {
	for name, value := range map[string]string{
		"SPX_IAM_PORT":          "invalid",
		"SPX_CONSOLE_IAM_TOKEN": "invalid",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := Load("../../config.yaml"); err == nil {
				t.Fatal("invalid supplied IAM client configuration accepted")
			}
		})
	}
}

// TestSharedIAMLocation 验证共享连接与本地监听独立，模块 YAML 只提供后备值。
func TestSharedIAMLocation(t *testing.T) {
	t.Setenv("SPX_IAM_HOST", "iam.corp")
	t.Setenv("SPX_IAM_PORT", "9443")
	t.Setenv("SPX_CONSOLE_BIND_HOST", "127.0.0.1")
	cfg, err := Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IAM.BaseURL != "http://iam.corp:9443" || cfg.App.Host != "127.0.0.1" {
		t.Fatal("shared target and listener were mixed")
	}
}
