package repository

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
)

// LockUsers 按统一 UUID 顺序锁定用户，登录轮换与管理变更遵循同一锁顺序。
// 生成的查询是单 id 形式，因此在 Go 侧排序后再逐条加锁，等价于 SQL 的 ORDER BY id。
func LockUsers(ctx context.Context, tx DB, ids []uuid.UUID) error {
	if err := requireTransaction(tx, "LockUsers"); err != nil {
		return err
	}

	ordered := slices.Clone(ids)
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	for _, id := range ordered {
		if err := queries(tx).LockUser(ctx, id); err != nil {
			return err
		}
	}

	return nil
}
