-- +goose Up
-- 收紧账号与邮箱约束，与 identity 包规则保持一致。项目未发版，按清空重建处理：
-- 直接替换约束，不为历史数据保留兼容层。
ALTER TABLE iam.users
 DROP CONSTRAINT users_username_check,
 ADD CONSTRAINT users_username_check CHECK(username ~ '^[A-Za-z][A-Za-z0-9_.-]{1,37}[A-Za-z0-9]$'),
 DROP CONSTRAINT users_email_check,
 ADD CONSTRAINT users_email_check CHECK(email IS NULL OR char_length(email) BETWEEN 3 AND 254);

-- 用户名不得包含连续分隔符；与 identity.NormalizeUsername 的拒绝规则一致。
ALTER TABLE iam.users
 ADD CONSTRAINT users_username_no_consecutive_separators_check
 CHECK(username !~ '[._-]{2,}');

COMMENT ON CONSTRAINT users_username_check ON iam.users IS
  '用户名 3–39 位，字母开头、字母数字结尾，中间允许 . _ -；与 identity.NormalizeUsername 一致。';

COMMENT ON CONSTRAINT users_username_no_consecutive_separators_check ON iam.users IS
  '拒绝 a--b / a..b 之类的歧义标识；与 identity.NormalizeUsername 一致。';

-- 邮箱唯一性键：小写归一的生成列 + 部分唯一索引。空邮箱表示未设置，不参与唯一性。
ALTER TABLE iam.users ADD COLUMN email_normalized text GENERATED ALWAYS AS (lower(email)) STORED;

CREATE UNIQUE INDEX users_email_normalized_key ON iam.users(email_normalized) WHERE email_normalized IS NOT NULL;

COMMENT ON COLUMN iam.users.email_normalized IS
  '邮箱唯一性键（小写）。不剥离加号子地址，不剥离点号：前者合法，后者仅对 gmail 消费者账号成立。';

-- +goose Down
DROP INDEX IF EXISTS iam.users_email_normalized_key;
ALTER TABLE iam.users DROP COLUMN IF EXISTS email_normalized;

ALTER TABLE iam.users
 DROP CONSTRAINT IF EXISTS users_username_no_consecutive_separators_check,
 DROP CONSTRAINT users_username_check,
 ADD CONSTRAINT users_username_check CHECK(username ~ '^[A-Za-z][A-Za-z0-9_.-]{2,63}$'),
 DROP CONSTRAINT users_email_check,
 ADD CONSTRAINT users_email_check CHECK(char_length(email) <= 320);
