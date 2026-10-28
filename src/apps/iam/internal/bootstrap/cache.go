package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
)

// openCache 创建并验证当前 Redis 连接；失败时不遗留客户端连接。
func openCache(ctx context.Context, cfg config.Cache) (*redis.Client, error) {
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
		logging.From(ctx).Error("Redis 连接检查失败", "event", "dependency.failed", "dependency", "redis", "error", logging.Error(err))
		return nil, errors.New("cache unavailable")
	}

	return client, nil
}
