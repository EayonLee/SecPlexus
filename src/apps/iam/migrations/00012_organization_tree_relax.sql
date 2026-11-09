-- +goose Up
-- 取消组织深度硬上限，改为「不限深度 + 结构性约束（单父、禁环）+ 建议 ≤5 层」。
--
-- 依据：GitHub teams / Keycloak groups / AD・LDAP OU / K8s HNC 均不设深度上限，
-- 只保留单父与禁环；仅 GCP(10)/AWS(5)/GitLab(20) 设硬顶。原 3 层上限比这些系统都严。
--
-- depth 列类型是 smallint（上限 32767）。完全取消 CHECK 等于放弃溢出兜底，
-- 因此保留一个远高于任何真实用量的宽松上界 1000。

-- 重建触发器：只去掉深度上限判断，保留 depth 计算与环检测。
-- 注意：不使用 pg_trigger_depth()（在 deferred trigger 下不可靠）；
-- reparent 的后代 depth 重算由服务层在同一事务内完成。
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION iam.organization_tree_guard() RETURNS trigger LANGUAGE plpgsql AS $$
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

ALTER TABLE iam.organizations DROP CONSTRAINT organizations_depth_check;
ALTER TABLE iam.organizations
  ADD CONSTRAINT organizations_depth_check CHECK(depth BETWEEN 0 AND 1000);

-- 递归查询（祖先链、子树、环检测）依赖 parent_id。原 organizations_single_root
-- 是部分索引（谓词 parent_id IS NULL），对 WHERE parent_id = $1 不可用，实测 Seq Scan。
CREATE INDEX organizations_parent_id ON iam.organizations(parent_id);

COMMENT ON COLUMN iam.organizations.depth IS
  '树深度，0 为根。由 organization_tree_guard 触发器维护，不可直接写入。不设业务上限，仅 CHECK(depth BETWEEN 0 AND 1000) 兜底 smallint 溢出。';

COMMENT ON COLUMN iam.organizations.parent_id IS
  '父组织；NULL 表示根。全局仅允许一个根，单父、禁止成环，不设深度业务上限。';

-- +goose Down
-- 回滚会重新施加 3 层上限；若已存在 depth>2 的组织则必须先处理，否则 CHECK 会失败。
-- 这里显式检测并给出可读错误，避免迁移以晦涩的约束违例失败。
-- +goose StatementBegin
DO $$
DECLARE deep int;
BEGIN
  SELECT count(*) INTO deep FROM iam.organizations WHERE depth > 2;
  IF deep > 0 THEN
    RAISE EXCEPTION 'cannot restore 3-level depth cap: % organization(s) deeper than level 3 exist; delete or reparent them first', deep;
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE iam.organizations DROP CONSTRAINT IF EXISTS organizations_depth_check;
ALTER TABLE iam.organizations
  ADD CONSTRAINT organizations_depth_check CHECK(depth BETWEEN 0 AND 2);

DROP INDEX IF EXISTS iam.organizations_parent_id;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION iam.organization_tree_guard() RETURNS trigger LANGUAGE plpgsql AS $$
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

COMMENT ON COLUMN iam.organizations.depth IS
  '树深度，0 为根。由 organization_tree_guard 触发器维护，不可直接写入。';
