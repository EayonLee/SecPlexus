package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Open 用配置创建并验证 Redis 连接。
//
// 每个模块都需要同一套超时与失败语义，抽在这里避免各写一份后逐渐分叉。
// 失败时不遗留客户端连接；调用方只负责在成功时关闭。
func Open(ctx context.Context, cfg Config) (*redis.Client, error) {
	options, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, errors.New("invalid cache connection configuration")
	}

	options.DialTimeout = 3 * time.Second
	options.ReadTimeout = 2 * time.Second
	options.WriteTimeout = 2 * time.Second
	options.PoolTimeout = 3 * time.Second
	options.MaxRetries = -1

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, errors.New("cache unavailable")
	}

	return client, nil
}
