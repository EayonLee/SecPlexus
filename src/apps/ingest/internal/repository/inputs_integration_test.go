//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

// 回归：创建 http 接入点返回一次性明文令牌，库里只留摘要。
func TestCreateInputReturnsTokenOnce(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "http")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, name) })

	created, token, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: name, Protocol: "http",
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if token == "" {
		t.Fatal("没有返回明文令牌，设备无法接入")
	}
	if created.ID == "" || created.Protocol != "http" || !created.Enabled {
		t.Fatalf("返回值不完整: %#v", created)
	}

	// 列表与更新都不得再出现明文，也不能存明文。
	page, err := repository.NewAdminStore(pool).ListInputPage(ctx, repository.InputListFilter{Limit: 20, ExactName: name})
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatal("列表返回的标识不一致")
	}

	var stored []byte
	if err = pool.QueryRow(ctx, `SELECT configuration->>'token_sha256' FROM ingest.inputs WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatalf("读取摘要失败: %v", err)
	}
	if len(stored) != 64 {
		t.Fatalf("库里存的不是 32 字节摘要，长度 %d", len(stored))
	}
}

// 重复名称与端口冲突要有可区分的错误，而不是统一的 500。
func TestCreateInputClassifiesConflicts(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "dup")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, name) })

	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{Name: name, Protocol: "http"}); err != nil {
		t.Fatalf("第一次创建失败: %v", err)
	}

	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{Name: name, Protocol: "http"}); !errors.Is(err, repository.ErrDuplicateName) {
		t.Fatalf("重名错误 = %v，期望 ErrDuplicateName", err)
	}

	port := int32(6100 + (int32(len(name)) % 200))
	firstName := uniqueName(t, "p1")
	secondName := uniqueName(t, "p2")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = ANY($1)`, []string{firstName, secondName})
	})

	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: firstName, Protocol: "syslog", SyslogPort: &port,
	}); err != nil {
		t.Fatalf("创建第一个 syslog 失败: %v", err)
	}

	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: secondName, Protocol: "syslog", SyslogPort: &port,
	}); !errors.Is(err, repository.ErrPortTaken) {
		t.Fatalf("端口冲突错误 = %v，期望 ErrPortTaken", err)
	}
}

// http 缺令牌不应由应用层兜底：数据库约束就拒绝，仓储层翻译成参数错误。
func TestCreateInputRejectsInvalidCombination(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "bad")
	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: name, Protocol: "syslog",
	}); !errors.Is(err, repository.ErrInvalidCombination) {
		t.Fatalf("错误 = %v，期望 ErrInvalidCombination", err)
	}
}

// 删除与修改：删除不存在的返回 NotFound；换令牌后旧令牌摘要立刻不同。
func TestUpdateAndDeleteInput(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "upd")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, name) })

	created, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{Name: name, Protocol: "http", Vendor: "Old vendor", Product: "Edge"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var before string
	if err = pool.QueryRow(ctx, `SELECT configuration->>'token_sha256' FROM ingest.inputs WHERE id = $1`, created.ID).Scan(&before); err != nil {
		t.Fatalf("读取摘要失败: %v", err)
	}

	vendor := "New vendor"
	updated, newToken, err := repository.UpdateInput(ctx, pool, created.ID, repository.UpdateInputParams{RotateToken: true, Vendor: &vendor})
	if err != nil {
		t.Fatalf("换令牌失败: %v", err)
	}
	if updated.Vendor != vendor || updated.Product != "Edge" {
		t.Fatalf("只改厂商后得到 %q / %q", updated.Vendor, updated.Product)
	}
	if newToken == "" {
		t.Fatal("换令牌没有返回新明文")
	}

	var after string
	if err = pool.QueryRow(ctx, `SELECT configuration->>'token_sha256' FROM ingest.inputs WHERE id = $1`, created.ID).Scan(&after); err != nil {
		t.Fatalf("读取新摘要失败: %v", err)
	}
	if string(before) == string(after) {
		t.Fatal("换令牌后摘要没变，旧令牌仍可用")
	}

	if err = repository.DeleteInput(ctx, pool, created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if err = repository.DeleteInput(ctx, pool, created.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("重复删除错误 = %v，期望 ErrNotFound", err)
	}
}
