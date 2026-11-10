-- 用户级行锁：登录轮换与管理变更都按同一 UUID 顺序加锁，避免死锁。
-- 单 id 形式由调用方按 id 升序逐条加锁，等价于在 SQL 里 ORDER BY id。
-- name: LockUser :exec
SELECT id FROM iam.users WHERE id = $1::uuid FOR UPDATE;
