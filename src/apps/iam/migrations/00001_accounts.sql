-- +goose Up
-- IAM 基线：账号、角色、权限、会话与审计。
--
-- 项目未发版，历史迁移已合并为这一份基线；已初始化旧基线的环境通过后续编号迁移前向修正，禁止手工删除 schema 或版本表。
-- 授权只有一条路径：users --user_roles--> roles --role_permissions--> permissions。
-- 没有组织、租户、成员关系或作用域，因此也没有按作用域分支的授权判定。

CREATE SCHEMA IF NOT EXISTS iam;

CREATE TABLE iam.users (
 id uuid PRIMARY KEY,
 username text NOT NULL,
 username_normalized text NOT NULL UNIQUE CHECK(username_normalized=lower(username)),
 display_name text NOT NULL CHECK(char_length(display_name) BETWEEN 1 AND 100),
 email text CHECK(email IS NULL OR char_length(email) BETWEEN 3 AND 254),
 -- 邮箱唯一性键（小写）。不剥离加号子地址，不剥离点号：前者合法，后者仅对 gmail 消费者账号成立。
 email_normalized text GENERATED ALWAYS AS (lower(email)) STORED,
 status text NOT NULL CHECK(status IN ('active','disabled')),
 -- 配置托管的内置管理员：由启动配置写入，页面不可删除、不可停用、不可改密码。
 configured boolean NOT NULL DEFAULT false,
 -- 账号版本：改密、停用、改配置会让该用户所有会话立即失效。
 auth_epoch bigint NOT NULL DEFAULT 1 CHECK(auth_epoch>0),
 -- 展示时区偏好：NULL 未设置；utc/browser 为语义值；其他须为 IANA 时区名。不参与时间存储。
 timezone text CHECK(timezone IS NULL OR char_length(timezone)<=64),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(username ~ '^[A-Za-z][A-Za-z0-9_.-]{1,37}[A-Za-z0-9]'),
 -- 拒绝 a--b / a..b 之类的歧义标识；与 account.NormalizeUsername 一致。
 CHECK(username !~ '[._-]{2,}')
);

CREATE UNIQUE INDEX users_email_normalized_key ON iam.users(email_normalized) WHERE email_normalized IS NOT NULL;

-- 只允许一个配置托管管理员，避免两份配置各自声明一个超管。
CREATE UNIQUE INDEX users_single_configured ON iam.users(configured) WHERE configured;

CREATE TABLE iam.password_credentials (
 user_id uuid PRIMARY KEY REFERENCES iam.users(id) ON DELETE CASCADE,
 password_hash text NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT now(),
 failed_attempts integer NOT NULL DEFAULT 0 CHECK(failed_attempts>=0),
 locked_until timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);

-- 角色是全局的：名称唯一，不再有组织归属或业务代码。
CREATE TABLE iam.roles (
 id uuid PRIMARY KEY,
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100 AND char_length(btrim(name))>0),
 name_normalized text GENERATED ALWAYS AS (lower(btrim(name))) STORED,
 description text NOT NULL DEFAULT '' CHECK(char_length(description)<=1000),
 -- 内置角色：由启动配置写入并持有全部权限，页面只读、不可删除。
 builtin boolean NOT NULL DEFAULT false,
 created_by uuid REFERENCES iam.users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX roles_name_normalized_key ON iam.roles(name_normalized);
CREATE UNIQUE INDEX roles_single_builtin ON iam.roles(builtin) WHERE builtin;

-- 权限目录扁平：权限键就是唯一稳定标识，module.resource.action 由键派生，不再单独存列。
CREATE TABLE iam.permissions (
 permission_key text PRIMARY KEY CHECK(permission_key ~ '^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*){2}'),
 description text NOT NULL,
 risk_level text NOT NULL CHECK(risk_level IN ('normal','sensitive','high')),
 delegable boolean NOT NULL,
 requires_recent_auth boolean NOT NULL,
 -- active = 代码目录中存在；retired = 已从目录移除，保留历史授权但立即失效。
 status text NOT NULL CHECK(status IN ('active','retired')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE iam.role_permissions (
 role_id uuid NOT NULL REFERENCES iam.roles(id) ON DELETE CASCADE,
 permission_key text NOT NULL REFERENCES iam.permissions(permission_key),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(role_id,permission_key)
);

CREATE INDEX role_permissions_permission ON iam.role_permissions(permission_key,role_id);

-- 用户直接持有角色。删除角色前服务层要求先解除全部绑定，因此这里用 RESTRICT。
CREATE TABLE iam.user_roles (
 user_id uuid NOT NULL REFERENCES iam.users(id) ON DELETE CASCADE,
 role_id uuid NOT NULL REFERENCES iam.roles(id),
 created_by uuid REFERENCES iam.users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,role_id)
);

CREATE INDEX user_roles_role ON iam.user_roles(role_id);

-- 会话只保存认证状态与期限：来源信息属于审计，不在这里重复存一份。
CREATE TABLE iam.user_sessions (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES iam.users(id) ON DELETE CASCADE,
 token_hash text NOT NULL UNIQUE,
 auth_epoch bigint NOT NULL CHECK(auth_epoch>0),
 state text NOT NULL CHECK(state IN ('active','revoked')),
 authenticated_at timestamptz NOT NULL,
 reauthenticated_at timestamptz,
 absolute_expires_at timestamptz NOT NULL,
 idle_expires_at timestamptz NOT NULL,
 last_seen_at timestamptz NOT NULL,
 revoked_at timestamptz,
 revoke_reason text CHECK(char_length(revoke_reason)<=100),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(absolute_expires_at>authenticated_at),
 CHECK(idle_expires_at<=absolute_expires_at)
);

CREATE INDEX user_sessions_user_state ON iam.user_sessions(user_id,state);
CREATE INDEX user_sessions_absolute ON iam.user_sessions(absolute_expires_at);

CREATE TABLE iam.scs_sessions(token text PRIMARY KEY,data bytea NOT NULL,expiry timestamptz NOT NULL);
CREATE INDEX scs_sessions_expiry ON iam.scs_sessions(expiry);

-- 审计没有组织维度；目标用 target_type/target_id 表达。
CREATE TABLE iam.audit_events (
 id uuid PRIMARY KEY,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 actor_type text NOT NULL CHECK(actor_type IN ('user','system','deployment_operator')),
 actor_id uuid,
 actor_name text,
 action text NOT NULL,
 target_type text NOT NULL,
 target_id uuid,
 outcome text NOT NULL CHECK(outcome IN ('success','denied','failure')),
 reason_code text,
 request_id text NOT NULL,
 decision_id uuid,
 metadata jsonb NOT NULL DEFAULT '{}' CHECK(octet_length(metadata::text)<=8192)
);

CREATE INDEX audit_events_occurred ON iam.audit_events(occurred_at);
CREATE INDEX audit_events_actor_time ON iam.audit_events(actor_id,occurred_at);
CREATE INDEX audit_events_request ON iam.audit_events(request_id);

-- 审计是追加写事实：数据库层面拒绝改写；删除只能由保留期清理在显式事务里执行。
-- 清理通过 iam.audit_purge 事务变量显式开闸，普通请求路径无法触发删除。
-- +goose StatementBegin
CREATE FUNCTION iam.audit_events_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND coalesce(current_setting('iam.audit_purge', true), '') = 'on' THEN
    RETURN OLD;
  END IF;

  RAISE EXCEPTION 'audit events are append-only' USING ERRCODE = '42501';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_events_append_only_bu BEFORE UPDATE OR DELETE ON iam.audit_events
 FOR EACH ROW EXECUTE FUNCTION iam.audit_events_append_only();

COMMENT ON TABLE iam.roles IS
  '全局角色：只有 id、name、description、builtin 与权限；用户经 user_roles 直接持有。';

COMMENT ON TABLE iam.user_roles IS
  '用户与角色的直接绑定。权限只经由角色授予，没有继承或作用域。';

COMMENT ON COLUMN iam.users.configured IS
  '配置托管的内置管理员：由启动配置写入，不可删除、停用或在页面改密。';

COMMENT ON TABLE iam.audit_events IS
  '追加写审计：触发器拒绝 UPDATE；DELETE 仅在保留期清理事务中通过 iam.audit_purge 开闸。';

-- +goose Down
-- 不提供自动回滚：基线重建意味着整库重建，回退请从备份恢复。
SELECT 1;
