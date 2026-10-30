-- +goose Up
-- 由 PostgreSQL 原子校验现存数据；不截断、改名或删除不合规组织。
ALTER TABLE iam.organizations
 DROP CONSTRAINT organizations_slug_check,
 DROP CONSTRAINT organizations_name_check,
 ADD CONSTRAINT organizations_slug_check CHECK(slug ~ '^[a-z][a-z0-9_-]{1,28}[a-z0-9]$'),
 ADD CONSTRAINT organizations_name_check CHECK(char_length(name) BETWEEN 1 AND 30 AND char_length(btrim(name))>0 AND name !~ U&'[\0001-\001F\007F-\009F]');

-- +goose Down
-- 旧约束无法容纳下划线标识；回退前由约束校验阻止静默数据改写。
ALTER TABLE iam.organizations
 DROP CONSTRAINT organizations_slug_check,
 DROP CONSTRAINT organizations_name_check,
 ADD CONSTRAINT organizations_slug_check CHECK(slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
 ADD CONSTRAINT organizations_name_check CHECK(char_length(name) BETWEEN 1 AND 100);
