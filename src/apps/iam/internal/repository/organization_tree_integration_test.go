//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// rootID 读取迁移创建的根组织，供组织树用例复用。
func rootID(t *testing.T, store *repository.Store) string {
	t.Helper()

	var id string
	if err := store.Pool.QueryRow(context.Background(), `SELECT id::text FROM iam.organizations WHERE parent_id IS NULL`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	return id
}

// insertOrg 直接插入组织，深度由触发器计算。
func insertOrg(t *testing.T, store *repository.Store, slug, name string, parentID *string) string {
	t.Helper()

	id := uuid.NewString()
	if _, err := store.Pool.Exec(context.Background(), `INSERT INTO iam.organizations(id,slug,name,parent_id) VALUES($1,$2,$3,$4)`, id, slug, name, parentID); err != nil {
		t.Fatal(err)
	}

	return id
}

// 迁移必须留下唯一根组织 master，深度为 0。
func TestMigrationCreatesSingleRootMaster(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	var slug string
	var depth int32
	if err := store.Pool.QueryRow(ctx, `SELECT slug, depth FROM iam.organizations WHERE parent_id IS NULL`).Scan(&slug, &depth); err != nil {
		t.Fatal(err)
	}

	if slug != "master" || depth != 0 {
		t.Fatalf("root = (%q,%d), want (master,0)", slug, depth)
	}
}

// 触发器按父节点计算 depth；深度不再有业务上限。
func TestOrganizationTreeComputesDepthWithoutCap(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	parent := &root
	var last string
	for level := 1; level <= 6; level++ {
		last = insertOrg(t, store, "deep-"+string(rune('a'+level)), "第 "+string(rune('0'+level))+" 层", parent)
		parent = &last

		var depth int32
		if err := store.Pool.QueryRow(ctx, `SELECT depth FROM iam.organizations WHERE id=$1`, last).Scan(&depth); err != nil {
			t.Fatal(err)
		}

		if int(depth) != level {
			t.Fatalf("level %d depth = %d, want %d", level, depth, level)
		}
	}
}

// 环仍被拒绝：深度上限取消不影响环检测。
func TestOrganizationTreeRejectsCycle(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	child := insertOrg(t, store, "cycle-a", "环 A", &root)
	grand := insertOrg(t, store, "cycle-b", "环 B", &child)

	// 把 child 挂到它的后代 grand 之下，形成 child -> grand -> child 环。
	if _, err := store.Pool.Exec(ctx, `UPDATE iam.organizations SET parent_id=$1 WHERE id=$2`, grand, child); err == nil {
		t.Fatal("cycle was accepted")
	}
}

// 组织标识不可修改；名称可以修改。
func TestOrganizationSlugIsImmutable(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	id := insertOrg(t, store, "immutable", "不可改名", &root)

	if _, err := store.Pool.Exec(ctx, `UPDATE iam.organizations SET slug='renamed' WHERE id=$1`, id); err == nil {
		t.Fatal("slug update was accepted")
	}

	if _, err := store.Pool.Exec(ctx, `UPDATE iam.organizations SET name='新名称' WHERE id=$1`, id); err != nil {
		t.Fatalf("name update rejected: %v", err)
	}
}

// 组织树投影返回祖先路径。
func TestOrganizationTreeReturnsPath(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	child := insertOrg(t, store, "path-a", "路径A", &root)
	grand := insertOrg(t, store, "path-a-1", "路径A1", &child)

	items, truncated, err := repository.OrganizationTree(ctx, store.DB)
	if truncated {
		t.Fatal("unexpected truncation in small fixture")
	}
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range items {
		if item.ID != grand {
			continue
		}

		if len(item.Path) != 3 || item.Path[0] != "AISOC" || item.Path[1] != "路径A" || item.Path[2] != "路径A1" {
			t.Fatalf("path = %v, want [AISOC 路径A 路径A1]", item.Path)
		}

		return
	}

	t.Fatal("grandchild missing from tree projection")
}

// 根组织必须被识别，供删除前置检查使用。
func TestIsRootOrganization(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	child := insertOrg(t, store, "not-root", "非根", &root)

	if isRoot, err := repository.IsRootOrganization(ctx, store.DB, root); err != nil || !isRoot {
		t.Fatalf("root detected = %v, err = %v; want true", isRoot, err)
	}

	if isRoot, err := repository.IsRootOrganization(ctx, store.DB, child); err != nil || isRoot {
		t.Fatalf("child detected as root = %v, err = %v; want false", isRoot, err)
	}
}

// 删除组织按依赖顺序清理，审计记录留存。
func TestDeleteOrganizationRemovesDependentsButKeepsAudit(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	root := rootID(t, store)
	org := insertOrg(t, store, "deletable", "可删除", &root)

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "deletemember", "deletemember", "可删除用户", ""); err != nil {
		t.Fatal(err)
	}

	if err := repository.InsertOrganizationUser(ctx, store.DB, uuid.New(), org, userID); err != nil {
		t.Fatal(err)
	}

	auditID := uuid.New()
	if _, err := store.Pool.Exec(ctx, `INSERT INTO iam.audit_events(id,actor_type,action,target_type,outcome,request_id,org_id) VALUES($1,'user','iam.test','organization','success',$2,$3)`, auditID, uuid.NewString(), org); err != nil {
		t.Fatal(err)
	}

	if err := repository.DeleteOrganization(ctx, store.DB, org); err != nil {
		t.Fatal(err)
	}

	var orgCount, auditCount int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.organizations WHERE id=$1`, org).Scan(&orgCount); err != nil {
		t.Fatal(err)
	}

	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.audit_events WHERE id=$1`, auditID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}

	if orgCount != 0 {
		t.Fatalf("organization not deleted: %d remaining", orgCount)
	}

	if auditCount != 1 {
		t.Fatalf("audit event lost on delete: %d remaining", auditCount)
	}
}

// 用户名冲突映射为专用错误码，而不是通用 conflict。
func TestDuplicateUsernameMapsToUsernameTaken(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "dupuser", "dupuser", "重复用户", ""); err != nil {
		t.Fatal(err)
	}

	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "dupuser", "dupuser", "重复用户二", ""); err != repository.ErrUsernameTaken {
		t.Fatalf("duplicate username error = %v, want ErrUsernameTaken", err)
	}
}

// 邮箱冲突映射为专用错误码；大小写不同仍视为重复。
func TestDuplicateEmailMapsToEmailTaken(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	if _, err := store.Pool.Exec(ctx, `INSERT INTO iam.users(id,username,username_normalized,display_name,email,status) VALUES($1,'mailone','mailone','邮箱一','dup@example.com','active')`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	err := repository.InsertUser(ctx, store.DB, uuid.New(), "mailtwo", "mailtwo", "邮箱二", "DUP@example.com")
	if err != repository.ErrEmailTaken {
		t.Fatalf("duplicate email error = %v, want ErrEmailTaken", err)
	}
}
