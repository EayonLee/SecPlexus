package bootstrap

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	redisconn "github.com/EayonLee/SecPlexus/src/packages/go/redis"
)

// openRedis 创建并验证当前 Redis 连接；失败时不遗留客户端连接。
func openRedis(ctx context.Context, cfg redisconn.Config) (*redis.Client, error) {
	client, err := redisconn.Open(ctx, cfg)
	if err != nil {
		logging.From(ctx).Error("Redis connection failed", "event", "dependency.failed",
			"dependency", "redis", "error", logging.Error(err))

		return nil, err
	}

	return client, nil
}
