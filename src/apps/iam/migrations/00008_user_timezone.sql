-- +goose Up
-- 用户展示时区偏好。服务端始终以 UTC 存储和传输时间，该列只决定渲染时区。
-- 可空表示未设置，此时前端回落浏览器时区；合法值见 identity.ValidTimeZone。
ALTER TABLE iam.users ADD COLUMN timezone text CHECK(timezone IS NULL OR char_length(timezone)<=64);

COMMENT ON COLUMN iam.users.timezone IS
  '展示时区偏好：NULL 未设置；utc/browser 为语义值；其他须为 IANA 时区名。不参与时间存储。';

-- +goose Down
ALTER TABLE iam.users DROP COLUMN timezone;
