package httpchannel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrRateLimited 表示该接入点超出配额；调用方转 429。
var ErrRateLimited = errors.New("rate limited")

// Limiter 是每接入点的请求频率限制。
// 接口存在是为了让通道用假实现测试，Redis 只是其中一种实现。
type Limiter interface {
	// Allow 消耗一个配额；超额返回 ErrRateLimited。
	Allow(ctx context.Context, connectorID string) error
}

// tokenBucketScript 是原子令牌桶：读、补、扣在一次执行内完成，
// 避免"先读后写"在并发下超发。
//
// KEYS[1] 桶键；ARGV: 上限、每秒补充速率、当前毫秒、消耗量。
var tokenBucketScript = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])

local bucket = redis.call("HMGET", KEYS[1], "tokens", "stamp")
local tokens = tonumber(bucket[1])
local stamp = tonumber(bucket[2])

if tokens == nil then
  tokens = capacity
  stamp = now
end

local elapsed = math.max(0, now - stamp)
tokens = math.min(capacity, tokens + elapsed * rate / 1000)

if tokens < cost then
  redis.call("HSET", KEYS[1], "tokens", tokens, "stamp", now)
  redis.call("PEXPIRE", KEYS[1], 60000)
  return 0
end

tokens = tokens - cost
redis.call("HSET", KEYS[1], "tokens", tokens, "stamp", now)
redis.call("PEXPIRE", KEYS[1], 60000)
return 1
`)

// RedisLimiter 用 Redis 令牌桶实现限流。
// Redis 不可用时**拒绝**而不是放行：放行等于关掉暴力推送防护，与 IAM 登录限流同语义。
type RedisLimiter struct {
	client   *redis.Client
	capacity int
	rate     float64
	prefix   string
}

// NewRedisLimiter 创建限流器；capacity 是突发上限，rate 是每秒补充速率。
func NewRedisLimiter(client *redis.Client, capacity int, rate float64) (*RedisLimiter, error) {
	if client == nil {
		return nil, errors.New("redis client is required for rate limiting")
	}
	if capacity <= 0 || rate <= 0 {
		return nil, errors.New("rate limit capacity and rate must be positive")
	}

	return &RedisLimiter{client: client, capacity: capacity, rate: rate, prefix: "spx:ingest:limit:"}, nil
}

// Allow 消耗一个配额。
func (l *RedisLimiter) Allow(ctx context.Context, connectorID string) error {
	now := time.Now().UnixMilli()
	key := l.prefix + connectorID

	allowed, err := tokenBucketScript.Run(ctx, l.client, []string{key},
		l.capacity, l.rate, now, 1).Int()
	if err != nil {
		// 限流计数不可用时不放行：宁可拒绝，也不让防护失效。
		return fmt.Errorf("rate limiter unavailable: %w", err)
	}

	if allowed == 0 {
		return ErrRateLimited
	}

	return nil
}
