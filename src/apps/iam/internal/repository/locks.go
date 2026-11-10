package repository

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// LockUsers 按统一 UUID 顺序锁定用户，登录轮换与管理变更遵循同一锁顺序。
// 生成的查询是单 id 形式，因此在 Go 侧排序后再逐条加锁，等价于 SQL 的 ORDER BY id。
func LockUsers(ctx context.Context, tx *gorm.DB, ids []uuid.UUID) error {
	ordered := slices.Clone(ids)
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	queries := sqlcgen.New(tx.Statement.ConnPool)
	for _, id := range ordered {
		if err := queries.LockUser(ctx, id); err != nil {
			return err
		}
	}

	return nil
}
