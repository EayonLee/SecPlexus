# 运行日志与错误定位

Go 服务沿用标准 `log/slog`，通过 `src/packages/go/logging` 创建 JSON 日志和轮转文件；请求内用 `logging.From(ctx)` 自动带上 request_id。无需再安装另一套框架。日志级别、stdout 和轮转字段仍由模块 config.yaml 配置，本机默认 logs/iam.log、logs/console.log，容器使用 stdout。Web 使用 Next instrumentation，容器同样由运行平台收集 stdout。

## 从一条失败请求定位

1. 从 API Result 的 request_id 或 X-Request-ID 找到 `http.completed`。看 route、method、status 和 duration_ms，route 是注册模板，不含查询值。正常健康探针只在 Debug 输出。
2. IAM 管理操作看 `iam.management.completed` 的 operation、org_id、actor_id、target_id、phase 和 outcome。phase 区分锁、身份复核、授权、修改、权限版本、审计和提交；只有数据库提交成功才记录 committed。事务审计仍是数据库中的事实来源，运行日志不替代审计。
3. 有 `db.failed` 时看 error.sqlstate、error.constraint、error.table 和 source。source 是本项目函数/文件/行号，SQLSTATE 可区分唯一约束、外键、权限、连接或锁问题。`db.slow` 表示一次数据库操作至少一秒，不默认打印每次成功查询。
4. Console 内部调用看 `rpc.failed` 的 reason、code、status、upstream_request_id。transport 区分连接/TLS/超时，decode_result 等表示协议异常，authorization_rejected 是上游拒绝；用 upstream_request_id 到 IAM 查对应请求。上游标识仅作关联，不当身份凭据。
5. Web 服务端的 `web.request_failed` 包含框架 route/route_type、error_kind 与 digest；digest 可关联 Next 服务端错误记录。这个入口不记录请求对象或原始错误文本。Web 代理的 web.proxy_forwarded 表示已收到上游首部并转交响应流，不冒充流已发送完成；upstream_request_id 可关联 Go 请求。代理连接失败返回同一标识的 503 Result 和响应头。浏览器 APIError 保留 requestID；协议或网络故障输出受控阶段，非 JSON 401/503 仍正确失效身份或标记不可用。

例如，同一 request_id 下出现 `phase=append_audit`、`event=db.failed`、`error.sqlstate=42501`，表示审计写入遇到数据库权限问题；前端得到 internal_error，不会把数据库细节展示给用户。仅看见 500 或“执行失败”不算完整诊断。

## 核心覆盖点

| 入口 | 关键事件 | 诊断信息 |
|---|---|---|
| IAM / Console HTTP | http.completed、http.panic | 请求关联、匹配路由、状态/字节/耗时；panic 类型和代码栈位置 |
| 请求结构与来源 | http.validation_failed、security.csrf_rejected、security.origin_rejected | 契约操作/结果码或明确拒绝类别；不记录输入值 |
| 登录及二次认证 | auth.credentials_rejected、auth.login_succeeded、auth.reauthenticated | 失败类别；成功仅在 SCS 持久化之后记录用户和逻辑会话 ID |
| 会话加载/提交/补偿/撤销 | session.load_failed、session.commit_failed、session.compensation_failed、session.revoked | 失败阶段、数据库诊断、已验证资源标识；不记录 Cookie/token |
| 能力鉴权 | iam.authorization_allowed、iam.authorization_rejected | action、组织、决定标识、耗时；正常高频鉴权只在 Debug |
| 管理写操作 | iam.management.completed | 创建/修改用户、组织、成员、角色、授权及密码重置共用事务入口，记录阶段和提交结果 |
| 改密 | iam.password_changed | 更新凭据和撤销旧会话的事务已提交；不记录新旧密码 |
| PG / Redis | db.failed、db.slow、cache.failed、auth.rate_limited、dependency.failed | SQLSTATE、约束、代码位置；Redis 标准错误类别、限流桶类型/阈值/窗口，不记录账号/IP 原文或连接 URL |
| 内部 IAM 调用 | rpc.completed、rpc.failed | 请求和上游请求关联、能力/组织、受控失败阶段、TLS/网络错误类别 |
| Console SSE | sse.opened、sse.closed、sse.access_stopped | 用户/组织/决定、持续时间、撤权/重验失败/客户端断开；不打印每次心跳 |
| 启动、关闭与迁移 | service.stopping、service.listener_failed、migration.completed、migration.failed | 生命周期阶段、耗时和安全诊断；初始化/就绪失败仍失败退出或返回 503 |
| Next 服务端与代理 | service.started、web.request_failed、web.proxy_forwarded、web.proxy_failed | 框架路由类型、错误类型/digest、上游请求标识与连接失败类别；不记录 URL 查询 |
| 浏览器 API 边界 | api.failed、api.protocol_failed | 请求标识、HTTP 状态、网络/协议阶段；按统一 code 驱动登录失效和业务提示 |

HTTP 结果、业务结果和依赖诊断各有用途；不要在每一层反复打印相同的完整 error。正常查询、字段值、请求体和心跳不作为默认日志。

## 错误响应与敏感信息

普通 API 继续使用统一 Result。预期拒绝映射为现有 400/401/403/404/409/429；已识别的依赖故障为 503，未预期内部问题为 500。前端按 code/status 处理，message 仅展示。原始 cause 保留在服务端错误链，安全诊断只输出类型、受控原因、SQLSTATE/约束或系统错误码，禁止直接记录 err.Error()、数据库 Detail、SQL/参数、Cookie、Authorization、完整请求/响应、密码、账号输入或 DSN。

普通 JSON 在提交响应前遇到异常可返回统一失败；204 与 SSE 保留协议形式。响应已开始后只记录发送故障并结束，不能再拼一份 JSON。SCS 提交失败不发送成功 Cookie，逻辑登录补偿失败单独记录，需要按 request_id 和 session_id 排查。

日志可能包含稳定资源标识，按运维访问边界和保留策略管理。这里没有新增日志上传服务、追踪平台或前端埋点后端。

正常浏览器导航可能取消尚未完成的请求。确认请求上下文已取消时，IAM 和数据库层记录 Debug 取消事件；不将其归类为内部 500 或数据库故障。其他依赖超时、断连和真实服务错误仍按既定级别诊断。
