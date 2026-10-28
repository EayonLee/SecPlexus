//go:build integration

package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
)

// TestCacheACLConnection 验证结构化配置确实以指定 ACL 用户认证，而非回落到 default。
func TestCacheACLConnection(t *testing.T) {
	ctx := context.Background()
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		t.Fatal("需要独立测试 Redis，使用集成测试入口运行")
	}

	admin := redis.NewClient(options)
	t.Cleanup(func() { _ = admin.Close() })
	username := "spx_test_" + uuid.NewString()
	password := "cache @:/?" + uuid.NewString()
	digest := sha256.Sum256([]byte(password))
	if err = admin.Do(ctx, "ACL", "SETUSER", username, "reset", "on", "#"+hex.EncodeToString(digest[:]), "~spx:*", "+@all").Err(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := admin.Do(ctx, "ACL", "DELUSER", username).Err(); err != nil {
			t.Error("未能删除本次测试创建的 ACL 用户")
		}
	})

	// 配置构造连接的用户名、密码和转义需完整传递到实际 Redis 客户端。
	t.Setenv("SPX_REDIS_URL", "")
	t.Setenv("SPX_REDIS_USER", username)
	t.Setenv("SPX_REDIS_PASSWORD", password)
	host, port, err := net.SplitHostPort(options.Addr)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("SPX_REDIS_HOST", host)
	t.Setenv("SPX_REDIS_PORT", port)
	t.Setenv("SPX_REDIS_DB", strconv.Itoa(options.DB))
	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	client, err := openCache(ctx, cfg.Cache)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = client.Close() })
	actual, err := client.Do(ctx, "ACL", "WHOAMI").Text()
	if err != nil || actual != username {
		t.Fatal("没有使用配置的 ACL 用户")
	}

	connection, err := url.Parse(cfg.Cache.URL)
	if err != nil {
		t.Fatal(err)
	}

	connection.User = url.UserPassword(username, "incorrect-password")
	cfg.Cache.URL = connection.String()
	if rejected, err := openCache(ctx, cfg.Cache); err == nil {
		_ = rejected.Close()
		t.Fatal("错误 ACL 密码不应建立连接")
	}
}
