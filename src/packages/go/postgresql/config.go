// Package postgresql 提供各模块共同使用的 PostgreSQL 连接配置。
package postgresql

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Config 保存外部连接 URL 或自带实例的连接参数。
type Config struct {
	URL      string `yaml:"url"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// DSN 返回生效的连接地址，对自带实例的凭据执行 URL 转义。
func (c Config) DSN() string {
	if c.URL != "" {
		return c.URL
	}
	return (&url.URL{
		Scheme: "postgresql", User: url.UserPassword(c.User, c.Password),
		Host: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), Path: "/" + c.Database,
		RawQuery: "sslmode=disable",
	}).String()
}

// Validate 校验生效的连接参数，错误不包含凭据。
func (c Config) Validate() error {
	if c.URL == "" && (c.Host == "" || c.Port < 1 || c.Port > 65535 || c.Database == "" || c.User == "") {
		return errors.New("postgresql.host, postgresql.port, postgresql.database and postgresql.user must be set for the built-in connection")
	}
	if _, err := pgx.ParseConfig(c.DSN()); err != nil {
		return errors.New("postgresql.url must be a valid PostgreSQL connection string")
	}
	return nil
}
