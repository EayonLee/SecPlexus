# 扁平授权实施计划

状态：已实施（代码、迁移、构建与测试通过）；剩余项见「验证证据」中的待人工验收。

契约见 [spec.md](spec.md)。顺序按「先库、再后端、再前端、最后文档」，每一步都可独立验证。

| # | 事项 | 产物 | 验证 |
| --- | --- | --- | --- |
| 1 | 迁移基线合并 | `migrations/00001_accounts.sql` 单一基线；删除 00002–00016；schema 版本校验同步 | 空库迁移 + 集成测试 |
| 2 | 仓储层扁平化 | users/roles/permissions/role_permissions/user_roles/audit_events；删除旧范围表、范围树、授权快照 | `go build`、仓储集成测试 |
| 3 | 授权层扁平化 | `authorization.Snapshot{Principal, Permissions}`；`Evaluate` 单一路径；`CheckDelegation` 去 scope | 策略单测 |
| 4 | 服务与处理器 | 删除范围服务与关系服务；角色全局；用户列表带角色；`PUT /users/:id/roles`；`/authorize` 只收 action | `go vet`、handler 集成测试 |
| 5 | 内置数据同步 | 权限目录、超级管理员角色（builtin）、配置管理员（configured）在启动时幂等写入；内置角色/用户不可删改 | 启动集成测试 + 实测 |
| 6 | 路由与 Console | 扁平路由表；`/api/v1/console/{context,events}` | 路由测试、集成测试 |
| 7 | Web | 删除范围页、范围树与切换器；用户页平铺 + 角色抽屉；角色页内置行来自 API；权限常量 | `typecheck`、`lint`、`vitest`、`next build` |
| 8 | 配置与文档 | `config.yaml`（service_clients actions）、README、architecture、DESIGN 引用、plans 索引 | `config:check`、`catalogs:check` |
| 9 | 端到端验收 | 登录、用户/角色/审计三条主流程；内置数据只读 | Playwright + API 实测 |

## 关键决策

- **单基线迁移**：项目未发版，历史迁移直接合并；部署侧只需 `DROP SCHEMA iam CASCADE`（版本账本 `iam.goose_db_version` 在同一个 schema 内）后重跑。
- **超管身份即数据**：删除 `users.is_super_admin`，超管 = 持有内置「超级管理员」角色；授权判定不再有平台分支。
- **内置不可变**：`roles.builtin` 与 `users.configured` 由数据库标记，服务层拒绝改名/改权限/删除/停用。
- **无范围列的审计**：审计仍记录 `target_type/target_id`，不再有范围维度。

## 验证证据

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 空库迁移与内置数据 | `iam migrate`（全新数据库） | 通过；随后核对：权限 7、内置角色 1、内置角色权限 7、配置管理员 1、组织相关表 0 |
| Go 构建与静态检查 | `go build ./src/...`、`go vet` | 通过 |
| Go 单元与集成测试 | `go test ./src/apps/... ./src/packages/go/...`；`go test -tags integration ./src/apps/iam/... ./src/apps/console/...` | 全部通过；唯一失败为 `TestBackupRestoreRevokesRestoredCredentialsBeforeServing`，原因是本机没有 `pg_dump`（环境限制，与本次改动无关） |
| 不变量回归 | `go test -tags integration ./src/apps/iam/internal/service/`、`.../repository/` | `TestLastSuperAdminInvariant`、`TestConfiguredAdministratorRolesAreLocked`、`TestDeleteUserKeepsHistoricalRoleBindings`、`TestListUsersTreatsWildcardsLiterally` 通过 |
| 审计字段契约 | `TestFlatIdentityRoleAndUserLifecycle` 断言 | 审计项必须含 RFC3339 的 `occurred_at`，且不含 `time` / `org_id` |
| Web | `typecheck`、`lint`、`test:unit`、`next build` | 通过；单测 18 文件 / 58 用例；lint 仅 2 个既有 warning |
| 生成物与迁移边界 | `generate-catalogs.mjs --check`、`migrations-check.mjs` | 通过 |
| 生产库重建 | 删除 `iam` schema 与版本表后重跑迁移 | 通过；`status` 输出 `{"current_version":1,"pending":false,"target_version":1}` |

待人工验收：登录后的用户/角色/审计页面视觉与交互、Playwright 用例（需要重启本地栈）。

## 封板前的深度审查（行业证据对照）

对照 OWASP ASVS v5 / Session Management、NIST SP 800-63B、AWS/Entra 的托管角色模型、
CloudTrail 与 Azure SQL Ledger 的审计完整性、Google AIP-193 的错误契约，逐项核对本模块：

| 议题 | 业界口径 | 本模块处置 |
| --- | --- | --- |
| 会话超时 | 空闲与绝对超时都由服务端执行（ASVS 7.3.1/7.3.2） | 两者都在数据库判定，前端不可影响 |
| 会话轮换 | 认证/再认证后换新令牌（ASVS 7.2.4） | 登录与密码确认都轮换，旧记录标记 rotation |
| Cookie | `__Host-` + Secure + HttpOnly + SameSite | HTTPS 下自动使用 `__Host-spx_session` |
| 会话元数据 | 建议保留设备信息 | 无设备管理界面 → 删列，登录来源进审计 metadata |
| 近期认证 | GitHub sudo 2h、AWS MFA age、NIST AAL2 ≤1h | 30 分钟窗口，过期返回 `iam.recent_auth_required` 并自动重放 |
| 口令策略 | NIST：不设组合规则、不强制轮换、长度下限 | 12–128、无组合规则、无周期过期、Argon2id 64MiB/t=3/p=2 |
| 账号锁定 | OWASP/NIST 警告硬锁可被用于 DoS，倾向指数退避 | 5 次 / 15 分钟定时锁定 + 双维度限流；密码确认失败不计数 |
| 内置角色 | AWS 托管策略「调用方不可修改」，Entra 内置角色与自定义并列 | `roles.builtin` 只读、全局唯一、启动幂等重建 |
| 角色名唯一 | AWS 资源名同账号内唯一且大小写不敏感 | `UNIQUE(lower(btrim(name)))` |
| 审计完整性 | 同事务 + 追加写；CloudTrail/Ledger 用哈希链 | 同事务（写失败回滚）+ 触发器拒绝 UPDATE、DELETE 需显式开闸 |
| 审计保留 | 《网络安全法》≥6 个月 | 默认 365 天，分批清理 |
| 错误契约 | 稳定字符串码与 HTTP 状态解耦（AIP-193/Zalando） | `result.Code` 稳定字符串 + 信封 + request_id |
| PATCH 语义 | RFC 7396：缺省不变、null 删除 | 缺省保留、`""` 清空、`null` 拒绝（显式 null 无意义） |
| 引导管理员 | GitLab 一次性密码、Keycloak 临时账号、Grafana 仅首次 | **偏离**：用户明确要求「配置改了就同步」，故每次启动幂等同步并写审计 |

采纳的改进（本轮已实施）：权限键派生、retired 语义、审计追加写、限流进配置、
密码确认不锁定、Identity 类型化、LoginUser 与行锁投影拆分、ChangePassword 单事务、
删除未消费端点与三张表的冗余列。未采纳（记录为刻意决策）：哈希链审计、设备管理界面、
幂等键（`request_id` 只做关联）、cursor 分页（列表规模小且需要 total）。

## 风险

- 波及 IAM、Console、Web 三个应用与全部权限键，必须一次性切换；不保留旧路由别名。
- 现有数据库需要重建（仅保留部署侧配置），这是「无残留」的代价，已在 spec 中写明。
