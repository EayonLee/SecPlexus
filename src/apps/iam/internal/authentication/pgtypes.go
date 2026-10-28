package authentication

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// interval 按 PostgreSQL 间隔类型表达持续时间，单位为微秒。
func interval(duration time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: duration.Microseconds(),
		Valid: true}
}

// nullableText 将非空文本转换为可选数据库字段。
func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value,
		Valid: true}
}
