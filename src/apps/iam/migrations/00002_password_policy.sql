-- +goose Up
-- 删除首次改密状态和临时期限；保留现有密码哈希、身份版本和会话撤销能力。
ALTER TABLE iam.users DROP COLUMN must_change_password;
ALTER TABLE iam.password_credentials DROP COLUMN temporary_expires_at;

-- +goose Down
ALTER TABLE iam.users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
ALTER TABLE iam.password_credentials ADD COLUMN temporary_expires_at timestamptz;
