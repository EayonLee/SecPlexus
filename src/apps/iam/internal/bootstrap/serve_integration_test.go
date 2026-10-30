//go:build integration

package bootstrap

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// TestUnifiedHTTPListener 验证 HTTP 后端在外部 HTTPS 场景正常启动和退出。
func TestUnifiedHTTPListener(t *testing.T) {
	store := testutil.Database(t)
	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DB.URL = store.Pool.Config().ConnConfig.ConnString()
	cfg.Cache.URL = os.Getenv("SPX_TEST_REDIS_URL")
	cfg.Bootstrap.Username = "admin-" + uuid.NewString()
	cfg.Bootstrap.Password = "isolated administrator password"
	if err := store.Seed(context.Background(), cfg.Bootstrap.Username, cfg.Bootstrap.Password); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.CSRFKey = strings.Repeat("k", 32)
	cfg.ServiceClients = map[string]config.ServiceClient{"console": {Token: strings.Repeat("t", 32), Actions: []string{"iam.organizations.read"}}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.App.Host = "127.0.0.1"
	cfg.App.Port = listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	// 外部 HTTPS 与后端 HTTP 独立，不能据浏览器入口强制后端持有证书。
	cfg.Auth.WebOrigin = "https://console.example.com"
	cfg.Logging.File = ""
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("listener shutdown: %v", err)
		}
	})
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get("http://" + cfg.App.Addr() + "/health/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("HTTP listener did not become ready")
		case err := <-done:
			done <- err
			t.Fatal("service exited before readiness", err)
		case <-ticker.C:
		}
	}
}
