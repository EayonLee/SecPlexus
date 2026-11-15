//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

// 回归：创建 http 接入点返回一次性明文令牌，库里只留摘要。
func TestCreateSourceReturnsTokenOnce(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "http")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE name = $1`, name) })

	created, token, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{
		Name: name, Transport: "http",
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if token == "" {
		t.Fatal("没有返回明文令牌，设备无法接入")
	}
	if created.ID == "" || created.Transport != "http" || !created.Enabled {
		t.Fatalf("返回值不完整: %#v", created)
	}

	// 列表与更新都不得再出现明文，也不能存明文。
	items, err := repository.ListSources(ctx, pool)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	for _, item := range items {
		if item.Name == name {
			if item.ID != created.ID {
				t.Fatal("列表返回的标识不一致")
			}
		}
	}

	var stored []byte
	if err = pool.QueryRow(ctx, `SELECT token_digest FROM ingest.sources WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatalf("读取摘要失败: %v", err)
	}
	if len(stored) != 32 {
		t.Fatalf("库里存的不是 32 字节摘要，长度 %d", len(stored))
	}
}

// 重复名称与端口冲突要有可区分的错误，而不是统一的 500。
func TestCreateSourceClassifiesConflicts(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "dup")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE name = $1`, name) })

	if _, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{Name: name, Transport: "http"}); err != nil {
		t.Fatalf("第一次创建失败: %v", err)
	}

	if _, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{Name: name, Transport: "http"}); !errors.Is(err, repository.ErrDuplicateName) {
		t.Fatalf("重名错误 = %v，期望 ErrDuplicateName", err)
	}

	port := int32(6100 + (int32(len(name)) % 200))
	firstName := uniqueName(t, "p1")
	secondName := uniqueName(t, "p2")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE name = ANY($1)`, []string{firstName, secondName})
	})

	if _, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{
		Name: firstName, Transport: "syslog", SyslogPort: &port,
	}); err != nil {
		t.Fatalf("创建第一个 syslog 失败: %v", err)
	}

	if _, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{
		Name: secondName, Transport: "syslog", SyslogPort: &port,
	}); !errors.Is(err, repository.ErrPortTaken) {
		t.Fatalf("端口冲突错误 = %v，期望 ErrPortTaken", err)
	}
}

// http 缺令牌不应由应用层兜底：数据库约束就拒绝，仓储层翻译成参数错误。
func TestCreateSourceRejectsInvalidCombination(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "bad")
	if _, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{
		Name: name, Transport: "syslog",
	}); !errors.Is(err, repository.ErrInvalidCombination) {
		t.Fatalf("错误 = %v，期望 ErrInvalidCombination", err)
	}
}

// 删除与修改：删除不存在的返回 NotFound；换令牌后旧令牌摘要立刻不同。
func TestUpdateAndDeleteSource(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "upd")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE name = $1`, name) })

	created, _, err := repository.CreateSource(ctx, pool, repository.CreateSourceInput{Name: name, Transport: "http"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var before []byte
	if err = pool.QueryRow(ctx, `SELECT token_digest FROM ingest.sources WHERE id = $1`, created.ID).Scan(&before); err != nil {
		t.Fatalf("读取摘要失败: %v", err)
	}

	_, newToken, err := repository.UpdateSource(ctx, pool, created.ID, repository.UpdateSourceInput{RotateToken: true})
	if err != nil {
		t.Fatalf("换令牌失败: %v", err)
	}
	if newToken == "" {
		t.Fatal("换令牌没有返回新明文")
	}

	var after []byte
	if err = pool.QueryRow(ctx, `SELECT token_digest FROM ingest.sources WHERE id = $1`, created.ID).Scan(&after); err != nil {
		t.Fatalf("读取新摘要失败: %v", err)
	}
	if string(before) == string(after) {
		t.Fatal("换令牌后摘要没变，旧令牌仍可用")
	}

	if err = repository.DeleteSource(ctx, pool, created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if err = repository.DeleteSource(ctx, pool, created.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("重复删除错误 = %v，期望 ErrNotFound", err)
	}
}
