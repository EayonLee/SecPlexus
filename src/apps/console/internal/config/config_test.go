package config

import "testing"

func TestProvidedIAMConfigurationIsValidatedDuringLoad(t *testing.T) {
	for name, value := range map[string]string{
		"SPX_CONSOLE_IAM_BASE_URL": "http://invalid.example",
		"SPX_CONSOLE_IAM_TOKEN":    "invalid",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := Load("../../config.yaml"); err == nil {
				t.Fatal("invalid supplied IAM client configuration accepted")
			}
		})
	}
}
