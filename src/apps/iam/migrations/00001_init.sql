-- +goose Up
-- 说明：本模块尚未部署，迁移历史已合并为单条基线。
-- 合并规则：被后续迁移取代的中间态（旧闭集、二次修正）不再保留；
-- 内置数据（超级管理员、基线角色及其权限）由启动时的 Seed 幂等写入，不放进迁移。
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

-- IAM 基线表与列的说明。所有对象注释集中在这里，基线迁移保持结构定义。
-- 注释只描述业务含义、单位与约束，不重复字段名已经表达的信息。

COMMENT ON SCHEMA iam IS 'IAM 身份与授权模块。所有表只由 IAM 自己的迁移维护。';

COMMENT ON TABLE iam.users IS '账号主表：只存身份与访问状态，不含密码和会话。';
COMMENT ON COLUMN iam.users.id IS '账号唯一标识，UUID 由服务端生成。';
COMMENT ON COLUMN iam.users.username IS '登录名原样大小写，仅用于显示与审计。';
COMMENT ON COLUMN iam.users.username_normalized IS '登录名小写形式，唯一键；登录查找只使用这一列。';
COMMENT ON COLUMN iam.users.display_name IS '界面显示名称，1–100 个 Unicode 字符。';
COMMENT ON COLUMN iam.users.email IS '可选联系邮箱；NULL 表示未设置，空串不允许。';
COMMENT ON COLUMN iam.users.email_normalized IS '邮箱小写形式，用于唯一性比较；由 lower(email) 自动生成。';
COMMENT ON COLUMN iam.users.status IS '账号状态：active 可登录，disabled 停用并立即失效全部会话。';
COMMENT ON COLUMN iam.users.configured IS '是否为配置托管的内置管理员；全库最多一条，页面不可删除或停用。';
COMMENT ON COLUMN iam.users.auth_epoch IS '账号版本号，从 1 开始；改密、停用或改配置时递增，使旧会话立即失效。';
COMMENT ON COLUMN iam.users.timezone IS '展示时区偏好：NULL 未设置，utc/browser 为语义值，其他为 IANA 时区名；不参与时间存储。';
COMMENT ON COLUMN iam.users.created_at IS '创建时间，UTC。';
COMMENT ON COLUMN iam.users.updated_at IS '最后更新时间，UTC；由写入语句显式维护。';

COMMENT ON TABLE iam.password_credentials IS '账号密码凭据与登录锁定状态；与 users 一对一。';
COMMENT ON COLUMN iam.password_credentials.user_id IS '凭据所属账号；账号删除时级联删除。';
COMMENT ON COLUMN iam.password_credentials.password_hash IS 'Argon2id 密码哈希；永不出现在响应、审计或日志中。';
COMMENT ON COLUMN iam.password_credentials.changed_at IS '最近一次密码变更时间，UTC。';
COMMENT ON COLUMN iam.password_credentials.failed_attempts IS '当前锁定窗口内的连续失败次数，成功登录后清零。';
COMMENT ON COLUMN iam.password_credentials.locked_until IS '锁定截止时间，UTC；NULL 表示未锁定，到期后自动恢复尝试。';
COMMENT ON COLUMN iam.password_credentials.updated_at IS '最后更新时间，UTC。';

COMMENT ON TABLE iam.roles IS '全局角色表：只有名称、说明、内置标记与权限；用户经 user_roles 直接持有角色。';
COMMENT ON COLUMN iam.roles.id IS '角色唯一标识。';
COMMENT ON COLUMN iam.roles.name IS '角色名称，1–100 字符且不能全为空白；用于展示。';
COMMENT ON COLUMN iam.roles.name_normalized IS '角色名称小写去空白形式，唯一键；避免仅大小写不同的重名。';
COMMENT ON COLUMN iam.roles.description IS '角色说明，最多 1000 字符，默认空串。';
COMMENT ON COLUMN iam.roles.builtin IS '是否为内置角色；全库最多一条，持有全部权限且页面只读。';
COMMENT ON COLUMN iam.roles.created_by IS '创建者账号；创建者被删除时置空，保留角色本身。';
COMMENT ON COLUMN iam.roles.created_at IS '创建时间，UTC。';
COMMENT ON COLUMN iam.roles.updated_at IS '最后更新时间，UTC。';

COMMENT ON TABLE iam.permissions IS '权限目录投影：代码中的 permissions.json 是权威来源，启动时按 permission_key 同步。';
COMMENT ON COLUMN iam.permissions.permission_key IS '稳定权限键，固定为 module.resource.action；三段由键派生，不单独存列。';
COMMENT ON COLUMN iam.permissions.description IS '权限用途说明，来自代码目录。';
COMMENT ON COLUMN iam.permissions.risk_level IS '动作风险级别：normal 常规、sensitive 敏感、high 高风险。';
COMMENT ON COLUMN iam.permissions.delegable IS '是否允许操作者把该权限授予其他角色。';
COMMENT ON COLUMN iam.permissions.requires_recent_auth IS '执行前是否要求近期密码确认。';
COMMENT ON COLUMN iam.permissions.status IS '目录状态：active 仍在目录中，retired 已移除并立即失效但保留历史授权。';
COMMENT ON COLUMN iam.permissions.created_at IS '首次写入时间，UTC。';
COMMENT ON COLUMN iam.permissions.updated_at IS '最后同步时间，UTC。';

COMMENT ON TABLE iam.role_permissions IS '角色与权限的多对多绑定；权限只能经由角色授予。';
COMMENT ON COLUMN iam.role_permissions.role_id IS '被授予权限的角色；角色删除时级联删除。';
COMMENT ON COLUMN iam.role_permissions.permission_key IS '被授予的权限键，必须存在于 permissions。';
COMMENT ON COLUMN iam.role_permissions.created_at IS '授权时间，UTC。';

COMMENT ON TABLE iam.user_roles IS '用户与角色的直接绑定。权限只经由角色授予，没有继承或作用域。';
COMMENT ON COLUMN iam.user_roles.user_id IS '持有角色的账号；账号删除时级联删除绑定。';
COMMENT ON COLUMN iam.user_roles.role_id IS '被持有的角色；删除角色前服务层要求先解除全部绑定。';
COMMENT ON COLUMN iam.user_roles.created_by IS '分配该角色操作者账号；操作者被删除时置空。';
COMMENT ON COLUMN iam.user_roles.created_at IS '分配时间，UTC。';

COMMENT ON TABLE iam.user_sessions IS '可撤销的登录会话：只存认证状态与期限，来源信息属于审计。';
COMMENT ON COLUMN iam.user_sessions.id IS '逻辑会话标识，写入 SCS 会话数据用于撤权。';
COMMENT ON COLUMN iam.user_sessions.user_id IS '会话所属账号；账号删除时级联删除会话。';
COMMENT ON COLUMN iam.user_sessions.token_hash IS '会话令牌摘要，唯一；原始令牌只存在于浏览器 Cookie。';
COMMENT ON COLUMN iam.user_sessions.auth_epoch IS '签发时的账号版本；与账号当前版本不一致即视为失效。';
COMMENT ON COLUMN iam.user_sessions.state IS '会话状态：active 有效，revoked 已撤销且不能重新激活。';
COMMENT ON COLUMN iam.user_sessions.authenticated_at IS '首次认证时间，UTC；近期认证窗口以它为基准。';
COMMENT ON COLUMN iam.user_sessions.reauthenticated_at IS '最近一次密码确认时间，UTC；NULL 表示本次会话尚未确认过。';
COMMENT ON COLUMN iam.user_sessions.absolute_expires_at IS '绝对到期时间，UTC；重新认证也不延长。';
COMMENT ON COLUMN iam.user_sessions.idle_expires_at IS '空闲到期时间，UTC；每次请求续期但不超过绝对到期。';
COMMENT ON COLUMN iam.user_sessions.last_seen_at IS '最近一次活动时间，UTC。';
COMMENT ON COLUMN iam.user_sessions.revoked_at IS '撤销时间，UTC；NULL 表示未撤销。';
COMMENT ON COLUMN iam.user_sessions.revoke_reason IS '撤销原因代码，最多 100 字符；不含凭据或个人数据。';
COMMENT ON COLUMN iam.user_sessions.created_at IS '创建时间，UTC。';

COMMENT ON TABLE iam.scs_sessions IS 'SCS 组件自己的 Cookie 会话存储；由组件读写，业务代码不直接修改。';
COMMENT ON COLUMN iam.scs_sessions.token IS 'SCS 会话令牌，主键。';
COMMENT ON COLUMN iam.scs_sessions.data IS 'SCS 编码后的会话数据，包含逻辑会话标识等内部状态。';
COMMENT ON COLUMN iam.scs_sessions.expiry IS '过期时间，UTC；过期行由维护命令清理。';

COMMENT ON TABLE iam.audit_events IS '追加写审计：触发器拒绝 UPDATE；DELETE 仅在保留期清理事务中通过 iam.audit_purge 开闸。';
COMMENT ON COLUMN iam.audit_events.id IS '审计事件唯一标识。';
COMMENT ON COLUMN iam.audit_events.occurred_at IS '事件发生时间，UTC；列表按此列排序分页。';
COMMENT ON COLUMN iam.audit_events.actor_type IS '操作者类型：user 登录用户、system 系统自身、deployment_operator 运维命令。';
COMMENT ON COLUMN iam.audit_events.actor_id IS '操作者账号标识；系统事件可为空，不设外键以保留删除后的历史。';
COMMENT ON COLUMN iam.audit_events.actor_name IS '操作者名称快照，账号改名或删除后仍可追溯。';
COMMENT ON COLUMN iam.audit_events.action IS '稳定动作名称，例如 iam.auth.login 或 iam.users.manage。';
COMMENT ON COLUMN iam.audit_events.target_type IS '被操作资源类型，例如 user、session、role。';
COMMENT ON COLUMN iam.audit_events.target_id IS '被操作资源标识；资源删除后仍保留原值。';
COMMENT ON COLUMN iam.audit_events.outcome IS '操作结果：success 成功、denied 被拒绝、failure 执行失败。';
COMMENT ON COLUMN iam.audit_events.reason_code IS '稳定原因代码，例如 invalid_credentials；不含自由文本。';
COMMENT ON COLUMN iam.audit_events.request_id IS '关联 HTTP 请求标识，用于把日志与审计对齐。';
COMMENT ON COLUMN iam.audit_events.decision_id IS '授权决定标识，仅授权相关事件填写。';
COMMENT ON COLUMN iam.audit_events.metadata IS '经过筛选的扩展字段，JSONB 编码后不超过 8192 字节；禁止写入凭据。';

-- 审计列表改为 (occurred_at,id) 键集分页：排序键必须由同一个复合索引支撑，
-- 否则深翻会退化为排序加扫描。单列 occurred_at 索引被复合索引完全覆盖。
CREATE INDEX IF NOT EXISTS audit_events_occurred_id ON iam.audit_events(occurred_at, id);
DROP INDEX IF EXISTS iam.audit_events_occurred;

COMMENT ON INDEX iam.audit_events_occurred_id IS
  '审计列表排序与键集分页的支撑索引：(occurred_at,id) 同时服务时间范围过滤与向前游标。';

-- 常用筛选组合（结果 + 时间）单独建索引，避免只靠排序索引回表过滤。
CREATE INDEX IF NOT EXISTS audit_events_outcome_occurred_id ON iam.audit_events(outcome, occurred_at, id);

-- 补全审计目标类别：角色操作此前漏在闭集之外。
--
-- 00004 的闭集来自"数据库已有值 + 部分代码路径"，漏掉了 roles 服务写入的 role，
-- 结果是角色创建在 CI 上被 CHECK 拒绝。这说明闭集必须枚举**全部生产写入路径**，
-- 而不只是历史数据里出现过的值。
--
-- 这里用前向迁移而不是回改 00004：已应用迁移不可修改。

ALTER TABLE iam.audit_events
    ADD CONSTRAINT audit_events_target_type_check
    CHECK (target_type IN ('user','session','role','platform','input','mapping')) NOT VALID;

ALTER TABLE iam.audit_events VALIDATE CONSTRAINT audit_events_target_type_check;

COMMENT ON COLUMN iam.audit_events.target_type IS '审计目标类别，闭集：user 账号、session 会话、role 角色、platform 平台整体、input 数据接入点、mapping 解析映射。';

-- +goose Down
DROP SCHEMA IF EXISTS iam CASCADE;
