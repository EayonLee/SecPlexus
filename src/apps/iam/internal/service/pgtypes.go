package service

import "github.com/jackc/pgx/v5/pgtype"

// text 将原因代码转换为数据库可选文本。
func text(value string) pgtype.Text {
	return pgtype.Text{String: value,
		Valid: true}
}
