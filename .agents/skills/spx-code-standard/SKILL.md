---
name: spx-code-standard
description: Guide SecPlexus implementation, behavior-preserving simplification, and code-quality review across Go services, API contracts, and Web clients. Use for project engineering changes or explicit code-quality reviews; keep the scope within the current task and use existing authoritative rules.
---

# SecPlexus 工程规范与质量审查

先读根 AGENTS.md 和当前需求的 Spec/Plan，再定位受影响的实现、调用方与稳定测试。本文记录项目实际约定，不替代业务设计；发现文档与代码不一致时说明差距，不把候选设计当成现状。

## 选用模式

- **开发**：明确本次行为和边界，直接实现正常流程；只为真实复用、独立职责、I/O 或事务边界抽取函数/接口。先确认行为变更对应的测试失败，再实现和回归；不以整理为由修改无关模块。
- **审查**：用户要求质量审查时，默认只读指定 diff/范围，追踪必要调用方。按严重性给出文件行号、触发条件、影响和可执行建议；区分确定问题与待验证风险。修复、Git 写入或部署仍以用户授权为准。

## 目录与依赖

| 位置 | 职责 |
|---|---|
| `src/apps/<module>/cmd/main.go` | 读取启动参数、选择命令和调用启动入口 |
| `internal/config/` | 解析/校验 config.yaml，整理类型化配置；不建立数据库、缓存或 HTTP 连接，不执行迁移 |
| `internal/bootstrap/` | 显式创建依赖、组装用例与路由、启动及关闭；失败时清理已创建资源 |
| `internal/router/` | 集中注册和按业务分组路由；不查询数据库或复制权限规则 |
| `internal/handler/` | 绑定请求、调用用例、写 Result；IAM 管理接口在 `handler/management/`，不直接查询数据库 |
| `internal/service/` | 已有业务规则、权限/组织范围、事务与审计顺序 |
| `internal/repository/` | GORM 数据访问、必要的 PostgreSQL SQL、同一事务内的数据操作 |
| `internal/middleware/` | 模块专属中间件；IAM 的 Session、CSRF、契约验证与内部身份策略 |
| `migrations/` | 模块版本化 SQL 和 embed；保留已执行脚本、编号和数据 |
| `src/packages/go/` | 多服务实际共用的实现，不依赖应用私有包 |

按能力/业务拆目录和文件，相关职责放在一起。较大的独立业务可用子包；同一个服务类型的方法适合按用户、组织、成员等拆文件，不为目录整齐强拆成互相依赖的包。不要为每张表生成整套层、接口或空目录，不创建万能 util、全局依赖容器或基础 CRUD 框架。

## 请求、Result 与中间件

- 公共请求绑定和路由协议边界在 `src/packages/go/httpx/`；公共请求标识、异常恢复和访问日志在 `httpx/middleware/`；模块专属安全中间件留模块内。路由选择并挂载，不把所有能力塞进一个通用中间件。
- 结果封装在 `src/packages/go/result/`：`result.Success(w, data)`、`result.Created(w, data)`、`result.NoContent(w)`；`result.Fail(w)` 默认内部错误，`result.Fail(w, common.CodePermissionDenied)` 自动使用登记的状态和消息。
- 仅需特定公开提示时使用 `result.FailWithMessage`，不能传原始 error、SQL、密码或内部地址。新 code 登记在 `src/contracts/common/error-codes.json`，公共 Result schema 在同目录的 `openapi.json`；其他语言遵循同一 wire contract，不另造格式。
- 模块手写协议源为 `src/contracts/<module>/openapi.source.json`，`openapi.json`、Go/TS 类型和结果码是生成物。通过 `pnpm contracts:generate` 更新，不能手改生成物或让 Go struct 成为另一套协议真源。
- HTTP 状态必须表达真实成功/失败；204 和 SSE 等不套普通 Result。处理错误时保留原本的事务与资源语义，响应已开始后不能再写一份失败 JSON。
- 请求结构/schema 在拥有它的 HTTP 边界验证，配置同理；向内传递类型化值。内部仍须验证权限、组织范围、最新业务状态和并发版本，不以“入口验证过”删除这些检查。

## 数据、缓存与启动

- 当前 PostgreSQL 使用 GORM，升级使用 Goose；普通 CRUD 不手拼 SQL，锁、原子更新或复杂投影的必要 SQL 集中在 repository 并说明约束。业务变更与审计使用同一事务执行器，不在事务内悄悄回到根连接；SCS 的独立提交不能被描述为同一事务。
- 默认共用 PG/Redis 连接配置与账号，模块可明确覆盖；不另建迁移账号、实例 ID 或未使用的驱动注册框架。普通启动连接已有基础设施，Compose 是显式部署入口；配置方式查 `docs/configuration.md`。
- key 模板固定在 `src/packages/go/cache/key/`，按真实模块分文件，例如 `iam.go`。模板显式写 `spx:<模块>:<业务>:`，前缀固定，不提供环境变量/配置覆盖，禁止散落拼接或重复加 spx；保持 TTL、限流原子性和账号摘要语义。
- 创建者明确负责关闭连接、客户端、goroutine 和轮转日志；配置加载不做网络 I/O。依赖显式传递，context 用于取消、期限和请求数据，不用于查找应用服务。
- 开发用基础容器和数据卷是持久资源，普通停止只 stop，不删除容器、卷或数据。临时测试资源必须单独创建、精确识别和清理，不能把测试结束清理套用到开发环境；不执行全局 prune。

## 可读性

- 代码先让人读懂，再考虑缩短；错误尽早返回，避免无意义 helper、层层透传或为理论边界膨胀代码。小而清楚的函数不因只有一处调用就必须内联，单个简单流程也不硬拆十步。
- 方法/函数之间空一行，注释紧贴声明且与上个函数留空行；函数内按完整步骤分段，不把无关初始化、执行和清理挤成一块。较长调用、条件、SQL 和多字段结构按语义换行，不按固定行数拆函数。
- Go 导出类型、函数和方法使用中文文档注释，以名称开头；关键字段说明用途、单位、状态或约束，行尾注释由 gofmt 对齐。复杂内部逻辑解释原因和不变量，不逐行翻译；生成代码不为补注释而手改。
- 命名表达当前职责，遵循语言惯例；保留 ctx/err 等短作用域惯用名，不创造无意义缩写或重复别名。命名和排版示例查 `docs/plans/platform-service-architecture/code-style.md`。
- Web 使用Next.js / shadcn；组件用法由根目录的官方 `shadcn` Skill 负责。保持上游源码与视觉基准，项目逻辑放新增文件；本技能处理工程规范、代码简化和质量审查。

## 核心链路日志与异常

- 使用现有 slog 与 `logging.From(ctx)`，请求关联向下传递；每个接口追踪到实际用例及依赖边界，覆盖关键决策、事务阶段、依赖失败和最终结果。只写“失败”或只写入口日志不算可诊断，字段应能指出 action/资源范围/阶段/错误类别与代码位置；正常查询和心跳不逐条打印。
- 技术诊断使用 `logging.Error(err)`，保留错误链、SQLSTATE/约束、系统错误类别；禁止直接打印 err.Error()、panic 值、SQL 参数、请求/响应对象、Cookie、Token、密码、DSN 或数据库 Detail。业务拒绝用现有 code 统一映射，不能把故障吞成成功或向前端暴露内部错误。
- 日志按拥有的边界记录：依赖输出技术诊断，用例输出结果和阶段，HTTP 输出请求摘要；不逐层复制同一完整错误。成功只在对应持久化/提交之后记录，审计仍与业务在同一事务；SCS 独立提交失败必须拒绝成功并处理逻辑补偿。SSE 已提交后不能追加失败 JSON。
- 分段按“校验及锁定 → 授权/委派 → 修改 → 审计/提交”等完整步骤组织，中文注释写明为什么；长参数/日志字段调用分行。不能仅运行 gofmt 后声称拥挤代码已经整理，也不在每行之间机械插空行。覆盖及定位示例见 `docs/observability.md`。
- 镜像定义统一在 `deploy/docker`，使用仓库根 build context；编排引用这一份构建定义或产物，不在 Compose/K8s/模块下复制 Dockerfile。仅在实际构建边界不同后再拆分。

## 保持行为的简化

先读调用者、错误路径及测试，明确代码存在的原因。简化必须保持输入输出、错误类型、异步时序、查询顺序、持久化副作用、事务与资源释放；不理解授权或兼容边界时先查证，不能靠删除测试证明更简单。

- 默认具体类型和显式依赖；接口按调用方实际使用定义，只有一个实现也可能存在真实 I/O 隔离边界，但不为每个 struct 配一份 interface 或 Mock。
- 早返回展开错误路径；需要定位上下文时用 `%w` 保留 Go 错误链，不层层重复包装、记录同一个错误，不把错误吞成零值成功。普通错误不 panic。
- 不为顺序工作增加 goroutine/channel；确需并发时明确取消、错误传播和退出条件。循环内资源及时释放，避免无必要锁内 I/O；context 显式传递，不存进业务 struct 充当全局环境。
- React 派生状态不重复保存，hooks 承载真实共享状态/副作用；不要把每个调用再套一个 hook 或只透传的 helper。保留可读循环或 map/filter，不用嵌套 reduce、三元链和对象展开压缩复杂步骤。
- 类型转换、可选链和默认值不能掩盖未定义状态或认证失败。不能机械删除 async/await、try/finally、defer 或 context manager，它们可能决定异常与清理时序。
- 不因字段相同就合并 API DTO、领域状态和数据库模型；先确认敏感字段、校验、变更及序列化边界。泛型、反射、指针、builder 和函数选项都应有当前语义依据。
- 修改后检查调用链、概念数、命名与注释是否更清楚。没有具体复杂度问题就保留原样；缩短行数、固定重复次数或增加抽象层都不是独立目标。

## 验证与审查输出

测试入口和隔离标准见 `docs/testing.md`：单元覆盖规则/HTTP 边界，真实集成覆盖 PG/Redis、事务和组件契约，Playwright E2E 覆盖少量核心用户流程。复用稳定用例，不把临时复现逐层堆进长期集成/E2E，不以 Mock、安装依赖或格式通过冒充链路成功。

只对改动执行匹配检查，完整交付按需求计划验收。审查应关注真实影响：跨模块私表、handler SQL、重复校验、事务逃逸、Result/code/status 失配、Session 保存失败仍响应成功、流式包装、组织切换旧响应、资源泄漏和阅读困难。未发现问题也说明覆盖范围和未验证项，不以推测性风险或单纯格式偏好自动发起全仓重构。

## 当前框架接入

- HTTP使用原生Gin与OpenAPI生成路由；参考`docs/development/role-management.md`。复用官方扩展点，删除被替代的通用机制，不再接Chi/Authboss或回放HTTP认证请求。
- 配置用Viper显式环境绑定/mapstructure严格解码、Cobra命令；不恢复旧占位符解析。Schema/绑定/生成器规则及版本见`src/contracts/README.md`和Gin重构能力记录。
- Web服务器状态用Query，表单用RHF和生成Zod；生成SDK只消费公开契约。敏感凭据不入MutationCache；缓存键含身份/组织授权版本，切换先取消清空。
