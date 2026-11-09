-- +goose Up
-- 组织树：单父节点、深度上限 3（0–2）、全局唯一根 master。
-- 项目未发版，按清空重建处理：不保留旧组织 id，不做数据回填。
ALTER TABLE iam.organizations ADD COLUMN parent_id uuid REFERENCES iam.organizations(id);

-- depth 不设默认值：由触发器按 parent.depth+1 计算，避免遗漏导致 NULL/0 混用。
ALTER TABLE iam.organizations ADD COLUMN depth smallint;

-- 全局唯一根：部分唯一索引，只约束 parent_id IS NULL 的行。
CREATE UNIQUE INDEX organizations_single_root ON iam.organizations((parent_id IS NULL)) WHERE parent_id IS NULL;

-- 未发版：组织树引入前不存在层级语义，清空组织相关数据重建。
-- 账号、凭据与会话保留：Seed 会为根组织幂等地补齐管理员关系，
-- 既有账号身份不能因组织重建而丢失。
DELETE FROM iam.membership_roles;
DELETE FROM iam.role_permissions;
DELETE FROM iam.roles;
DELETE FROM iam.memberships;
DELETE FROM iam.organizations;

-- 根组织标识固定 master，名称可改。status 由后续迁移删除，此处仍写入以兼容列约束。
INSERT INTO iam.organizations(id, slug, name, status, parent_id, depth)
VALUES (gen_random_uuid(), 'master', 'AISOC', 'active', NULL, 0);

-- +goose StatementBegin
-- 触发器同时承担三项约束：计算 depth、限制深度、禁止成环。
-- 三者都依赖跨行读取，无法用 CHECK 表达。
CREATE FUNCTION iam.organization_tree_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_depth smallint; cursor_id uuid;
BEGIN
  IF NEW.parent_id IS NULL THEN
    NEW.depth := 0;
    RETURN NEW;
  END IF;

  SELECT depth INTO parent_depth FROM iam.organizations WHERE id = NEW.parent_id;
  IF parent_depth IS NULL THEN
    RAISE EXCEPTION 'parent organization not found' USING ERRCODE = '23503';
  END IF;

  NEW.depth := parent_depth + 1;
  IF NEW.depth > 2 THEN
    RAISE EXCEPTION 'organization depth exceeds limit' USING ERRCODE = '23514';
  END IF;

  cursor_id := NEW.parent_id;
  WHILE cursor_id IS NOT NULL LOOP
    IF cursor_id = NEW.id THEN
      RAISE EXCEPTION 'organization cycle detected' USING ERRCODE = '23514';
    END IF;
    SELECT parent_id INTO cursor_id FROM iam.organizations WHERE id = cursor_id;
  END LOOP;

  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER organization_tree_guard_biu
  BEFORE INSERT OR UPDATE ON iam.organizations
  FOR EACH ROW EXECUTE FUNCTION iam.organization_tree_guard();

-- 深度与父子一致性由触发器维护，此处仅补非空与范围兜底。
ALTER TABLE iam.organizations
  ALTER COLUMN depth SET NOT NULL,
  ADD CONSTRAINT organizations_depth_check CHECK(depth BETWEEN 0 AND 2),
  ADD CONSTRAINT organizations_root_depth_check CHECK((parent_id IS NULL) = (depth = 0));

-- 标识不可修改：slug 是 URL 与审计中的稳定标识。名称与父子关系不受此约束。
-- +goose StatementBegin
CREATE FUNCTION iam.organization_slug_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.slug IS DISTINCT FROM OLD.slug THEN
    RAISE EXCEPTION 'organization slug is immutable' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER organization_slug_immutable_bu
  BEFORE UPDATE ON iam.organizations
  FOR EACH ROW EXECUTE FUNCTION iam.organization_slug_immutable();

COMMENT ON COLUMN iam.organizations.parent_id IS
  '父组织；NULL 表示根。全局仅允许一个根，深度上限 3 层（0–2）。';

COMMENT ON COLUMN iam.organizations.depth IS
  '树深度，0 为根。由 organization_tree_guard 触发器维护，不可直接写入。';

-- +goose Down
DROP TRIGGER IF EXISTS organization_slug_immutable_bu ON iam.organizations;
DROP FUNCTION IF EXISTS iam.organization_slug_immutable();
DROP TRIGGER IF EXISTS organization_tree_guard_biu ON iam.organizations;
DROP FUNCTION IF EXISTS iam.organization_tree_guard();
DROP INDEX IF EXISTS iam.organizations_single_root;
ALTER TABLE iam.organizations
  DROP CONSTRAINT IF EXISTS organizations_root_depth_check,
  DROP CONSTRAINT IF EXISTS organizations_depth_check,
  DROP COLUMN IF EXISTS depth,
  DROP COLUMN IF EXISTS parent_id;
