package authentication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	cachekey "github.com/EayonLee/SecPlexus/src/packages/go/cache/key"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

var ErrUnavailable = errors.New("authentication dependency unavailable")

var ErrRateLimited = errors.New("too many authentication attempts")
var countAttempt = redis.NewScript(`local count=redis.call('INCR',KEYS[1]);if count==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end;return count`)

// Limiter 使用 Redis 限制登录尝试。
type Limiter struct {
	Client *redis.Client
}

// Check 使用 Redis 按客户端地址和账号限制登录；Redis 不可用时拒绝请求。
func (l *Limiter) Check(ctx context.Context, ip, account string) error {
	for _, rule := range []struct {
		template      cachekey.Template
		value         string
		limit, window int64
	}{{cachekey.IAMLoginIP, ip, 60, 60000}, {cachekey.IAMLoginAccount, account, 20, 900000}} {
		hash := sha256.Sum256([]byte(rule.value))
		key := rule.template.Format(hex.EncodeToString(hash[:]))
		count, err := countAttempt.Run(ctx, l.Client, []string{key}, rule.window).Int64()
		if err != nil {
			logging.From(ctx).Error("登录限流存储不可用", "event", "cache.failed", "operation", "iam.login.limit", "bucket", string(rule.template), "error", logging.Error(err))
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}

		if count > rule.limit {
			logging.From(ctx).Warn("登录频率超过限制", "event", "auth.rate_limited", "bucket", string(rule.template), "limit", rule.limit, "window_ms", rule.window)
			return ErrRateLimited
		}
	}

	return nil
}
