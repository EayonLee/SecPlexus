// Package cache 定义全平台共享的 Redis 连接配置及其派生与校验规则。
// 全平台共用一个 Redis 实例，模块用 key 前缀区分数据；连接串的派生规则只在这一处维护。
package cache

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// Config 描述 Redis 连接及 ACL 用户；URL 非空时优先使用 URL。
type Config struct {
	Host     string `yaml:"host" validate:"required"`        // 监听或连接地址。
	Port     int    `yaml:"port" validate:"gte=1,lte=65535"` // 监听或连接端口。
	User     string `yaml:"user"`                            // Redis ACL 用户名；默认使用 default。
	Password string `yaml:"password"`                        // Redis 连接密码。
	DB       int    `yaml:"db"`                              // Redis 逻辑数据库编号。
	URL      string `yaml:"url"`                             // 可选的完整连接地址；填了就替代上面的分项字段。
}

// ResolveURL 返回生效的连接串：URL 非空时原样返回，否则由分项字段派生。
// 派生不建立网络连接，也不改写自身；调用方在配置加载时把结果存回 URL 字段。
func (c Config) ResolveURL() string {
	if c.URL != "" {
		return c.URL
	}
	u := url.URL{
		Scheme: "redis",
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   "/" + strconv.Itoa(c.DB),
	}
	if c.User != "" || c.Password != "" {
		u.User = url.UserPassword(c.User, c.Password)
	}
	return u.String()
}

// Validate 校验数据库编号与连接串可解析性；不建立网络连接，错误不包含配置值。
func (c Config) Validate() error {
	if c.DB < 0 {
		return errors.New("cache.db must be nonnegative")
	}
	if _, err := redis.ParseURL(c.URL); err != nil {
		return errors.New("cache.url must be a valid Redis connection URL; check cache fields")
	}
	return nil
}
