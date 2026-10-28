package authentication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"

	cachekey "github.com/EayonLee/SecPlexus/src/packages/go/cache/key"
)

var ErrUnavailable = errors.New("authentication dependency unavailable")
var ErrRateLimited = errors.New("too many authentication attempts")

// Limiter 使用成熟的原子 Redis 存储，来源与账号分别计数。
type Limiter struct{ ip, account *limiter.Limiter }

// NewLimiter 在启动时加载组件脚本；存储不可用时启动失败。
func NewLimiter(client *redis.Client) (*Limiter, error) {
	store, err := redisstore.NewStoreWithOptions(client, limiter.StoreOptions{Prefix: cachekey.Prefix})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &Limiter{ip: limiter.New(store, limiter.Rate{Period: time.Minute, Limit: 60}), account: limiter.New(store, limiter.Rate{Period: 15 * time.Minute, Limit: 20})}, nil
}

// CheckIP 在读取凭据前限制来源，包括无效请求。
func (l *Limiter) CheckIP(ctx context.Context, ip string) error {
	return check(ctx, l.ip, cachekey.IAMLoginIP, ip)
}

// CheckAccount 限制已经规范化的账号，不把账号明文写入 Redis。
func (l *Limiter) CheckAccount(ctx context.Context, account string) error {
	return check(ctx, l.account, cachekey.IAMLoginAccount, account)
}

// Check 用于同时需要来源与账号桶的密码变更请求。
func (l *Limiter) Check(ctx context.Context, ip, account string) error {
	if err := l.CheckIP(ctx, ip); err != nil {
		return err
	}
	return l.CheckAccount(ctx, account)
}

func check(ctx context.Context, counter *limiter.Limiter, template cachekey.Template, value string) error {
	hash := sha256.Sum256([]byte(value))
	state, err := counter.Get(ctx, template.ComponentKey(hex.EncodeToString(hash[:])))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if state.Reached {
		return &RateLimitError{RetryAfter: max(1, state.Reset-time.Now().Unix())}
	}
	return nil
}

// RateLimitError 返回当前固定窗口还需等待的秒数。
type RateLimitError struct{ RetryAfter int64 }

func (e *RateLimitError) Error() string { return ErrRateLimited.Error() }

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }
