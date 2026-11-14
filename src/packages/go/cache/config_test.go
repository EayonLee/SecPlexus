package cache_test

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/packages/go/cache"
)

// ResolveURL 由分项字段派生连接串。
func TestResolveURLDerivesFromFields(t *testing.T) {
	got := cache.Config{Host: "127.0.0.1", Port: 6379, DB: 2}.ResolveURL()
	if got != "redis://127.0.0.1:6379/2" {
		t.Fatalf("derived URL = %q", got)
	}
}

// 凭据存在时进入 userinfo，特殊字符按 RFC 3986 编码。
func TestResolveURLIncludesCredentials(t *testing.T) {
	got := cache.Config{Host: "cache.example", Port: 6380, DB: 3, User: "acl-user", Password: "p@ss"}.ResolveURL()
	if got != "redis://acl-user:p%40ss@cache.example:6380/3" {
		t.Fatalf("derived URL with credentials = %q", got)
	}
}

// 无凭据时不产生 userinfo 段。
func TestResolveURLOmitsCredentialsWhenAbsent(t *testing.T) {
	got := cache.Config{Host: "127.0.0.1", Port: 6379}.ResolveURL()
	if got != "redis://127.0.0.1:6379/0" {
		t.Fatalf("derived URL without credentials = %q", got)
	}
}

// 显式 URL 优先，不被分项字段覆盖。
func TestResolveURLKeepsExplicitURL(t *testing.T) {
	c := cache.Config{Host: "ignored", Port: 1, User: "ignored-user", URL: "redis://acl-user:encoded%40password@cache.example:6380/3"}
	if got := c.ResolveURL(); got != c.URL {
		t.Fatalf("explicit URL was overridden: %q", got)
	}
}

// 负的数据库编号必须被拒绝。
func TestValidateRejectsNegativeDB(t *testing.T) {
	if err := (cache.Config{DB: -1, URL: "redis://127.0.0.1:6379/0"}).Validate(); err == nil {
		t.Fatal("negative database accepted")
	}
}

// 连接串格式错误必须被拒绝；错误不包含配置值。
func TestValidateRejectsUnparseableURL(t *testing.T) {
	for _, value := range []string{"http://cache", "redis://cache/not-a-database", "redis://cache:wrong", ""} {
		t.Run(value, func(t *testing.T) {
			if err := (cache.Config{URL: value}).Validate(); err == nil {
				t.Fatalf("invalid URL accepted: %q", value)
			}
		})
	}
}

// 解析后的配置整体通过校验。
func TestValidateAcceptsResolvedConfig(t *testing.T) {
	c := cache.Config{Host: "127.0.0.1", Port: 6379, User: "default", Password: "secret"}
	c.URL = c.ResolveURL()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
