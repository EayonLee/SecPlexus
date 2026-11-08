package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/bootstrap"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/config"
)

func writeConfig(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "app:\n  host: 127.0.0.1\n  port: 18080\niam:\n  host: 127.0.0.1\n  port: 8081\n  token: " + token + "\nlogging:\n  level: error\n  stdout: true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// 凭证不合法时必须在打开监听前失败，否则服务会以无效凭证对外监听。
func TestServeRejectsInvalidCredentialBeforeListening(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, "too-short"))
	if err == nil {
		t.Fatal("Load accepted a short service credential")
	}

	if !strings.Contains(err.Error(), "SPX_CONSOLE_IAM_TOKEN") {
		t.Fatalf("unexpected error %v", err)
	}

	// 已拒绝的配置不应能让 Serve 继续；用零值确认它同样在监听前失败。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := bootstrap.Serve(ctx, cfg); err == nil {
		t.Fatal("Serve accepted a configuration without a valid IAM client")
	}
}

// 合法配置但上下文立即取消时，Serve 必须返回而不是永久阻塞。
func TestServeReturnsWhenContextCancelled(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, strings.Repeat("c", 32)))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- bootstrap.Serve(ctx, cfg) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}
