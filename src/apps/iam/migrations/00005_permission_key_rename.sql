-- +goose Up
-- 把 iam.permissions 的主键列从 key 改名为 permission_key。
-- 原因：key 是 SQL 保留字，部分方言要求加引号；且与 role_permissions.permission_key 命名不一致。
ALTER TABLE iam.permissions RENAME COLUMN key TO permission_key;

-- PostgreSQL 不会因列改名而更新 plpgsql 函数体，必须重建该触发器函数，
-- 否则 role_permissions 的每次写入都会因 "column key does not exist" 失败。
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION iam.check_organization_permission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM iam.permissions WHERE permission_key=NEW.permission_key AND scope='organization' AND status='active') THEN
  RAISE EXCEPTION 'only active organization permissions can be granted' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION iam.check_organization_permission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM iam.permissions WHERE key=NEW.permission_key AND scope='organization' AND status='active') THEN
  RAISE EXCEPTION 'only active organization permissions can be granted' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
ALTER TABLE iam.permissions RENAME COLUMN permission_key TO key;
