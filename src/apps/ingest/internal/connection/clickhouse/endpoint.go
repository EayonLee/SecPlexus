package clickhouse

import (
	"errors"
	"net/url"
	"regexp"
)

// ParseEndpoint 校验事件存储 HTTP 端点；认证信息由调用方单独注入。
func ParseEndpoint(address string) (*url.URL, error) {
	endpoint, err := url.Parse(address)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("invalid ClickHouse endpoint")
	}
	return endpoint, nil
}

var databaseName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// DatabaseNameValid 校验允许用作事件数据库的稳定 SQL 标识。
func DatabaseNameValid(name string) bool { return databaseName.MatchString(name) }
