# 后端工程结构与命名规范

本规范适用于 `src/apps/` 下所有模块（当前 `iam`、`console`，后续模块同样适用）以及
`src/packages/go` 的共享代码。目标是让新增模块有固定位置、固定依赖方向、固定命名，
评审时不需要重新讨论"代码该放哪儿"。

依据见文末[依据](#依据)，主要参考 Go 官方模块布局文档与大型 Go 项目（Kubernetes、
Prometheus、Grafana、moby、Terraform、Thanos）的目录实践。

## 1. 仓库级布局

```text
src/
  apps/<module>/            可独立启动的服务，一个模块一个进程边界
    cmd/                    进程入口：参数解析、配置加载、选择命令
    internal/               模块私有实现，Go 编译器强制不可被其他模块导入
    migrations/             版本化 SQL 与 embed 声明
    config.yaml             模块配置（唯一默认值来源）
    README.md               业务与技术说明、接口契约
  packages/go/<pkg>/        真实跨模块复用的代码（logging、result、httpx、iamclient…）
scripts/                    仓库级工具：启动、构建、迁移检查、测试编排
docs/                       设计与开发规范
deploy/                     部署描述
```

规则：

- `src/apps` 之间不互相 import；跨模块协作走 HTTP + `src/packages/go/*client`。
- 只有**已被两个以上模块使用**的代码才允许进 `src/packages`；单模块用的东西留在
  `internal`。提前抽象比重复更贵。
- 不新增顶层 `pkg/`；私有性由 `internal` 表达，复用性由 `src/packages` 表达。
- 生成文件由脚本产出并在文件头标注，禁止手改。

## 2. 模块内分层

```text
cmd/                       进程边界：cobra 命令、配置、信号、退出码
internal/
  router/                  路由注册与每个路由的认证方式
  handler/                 协议边界：绑定、DTO、结果码映射
  service/                 用例与事务边界：权限校验、不变量、审计
  repository/              持久化：SQL、行锁、迁移、连接池
  authorization/           授权规则（纯函数，不依赖 HTTP/DB）
  authentication/          会话、凭据、限流
  account/                 账号值对象校验与归一化
  middleware/              会话、CSRF、来源、输入边界
  model/                   HTTP 输入输出结构
  enum/                    数据库封闭取值的 Go 常量
  config/                  类型化配置加载与校验
  bootstrap/               依赖装配、就绪检查、生命周期
  testutil/                集成测试自有资源
```

依赖方向只能自上而下：

```text
cmd → bootstrap → router → handler → service → repository
                    ↘ middleware ↘ authentication ↘ account
authorization / enum / model 是被依赖的叶子包，不反向依赖任何上层
```

禁止：

- handler 里直接写 SQL，或 repository 返回 HTTP 结构。
- service 依赖 `*gin.Context`；它只接受 typed 值（`context.Context` + 值对象）。
- service 依赖 `internal/model`（HTTP DTO）。用例输入定义在 `service/input.go`，
  字段已解析成领域取值（例如 `enum.UserStatus`）；handler 负责绑定 DTO 并映射到用例输入。
  这样换一种入口（CLI、消息队列）不需要改业务代码，测试也不必构造 HTTP 结构。
- 跨越两个模块访问对方的 `internal`。
- 为"将来可能复用"提前抽包、提前加接口。

## 3. 命名

| 对象 | 规则 |
|---|---|
| 包 | 小写单数、无下划线、不用 `util`/`common`/`helpers` | 
| 文件 | 按职责命名，不用 `helpers.go`；同一职责可拆 `<topic>_queries.go` |
| 类型 | 名词；接口以 `-er` 结尾且只声明真实边界 |
| 函数 | 动词开头；`Create`/`Update`/`Delete`/`Resolve`/`Sync` 表达副作用 |
| 参数/变量 | 表达含义而非类型；`userID` 而不是 `id2` |
| 错误 | 哨兵错误 `ErrXxx`；包装用 `%w` 并说明上下文 |
| 常量 | `PascalCase`；封闭取值集中在 `internal/enum` |
| 测试 | `Test<行为>`，子测试用用户可观察的行为描述 |

禁止缩写堆叠（`usrMgr`、`prmSvc`）。名字长度随作用域增大而增加；三行的循环变量可以是 `i`，
包级导出名必须自解释。

## 4. 错误与边界

- 边界解析一次：HTTP 绑定在 `middleware`/`httpx` 完成，内部只接收 typed 值。
- 哨兵错误跨层传递，映射集中在一处（`handler.writeError`），不层层重复 `if err != nil { log } `。
- 不吞错：要么返回，要么记录并转换；不允许"记录后继续成功"。
- 事务边界在 service；repository 方法接收调用方传入的连接/事务，不自行开事务。迁移期这个参数仍是 `*gorm.DB`，目标形态是 pgx 池 + `sqlcgen`（见 [数据访问规范](data-access.md)）。
- 幂等：启动同步、迁移、维护命令重复执行后状态一致。

## 5. 日志与审计

- 结构化 `slog`，经 `logging.From(ctx)`；事件名 `<domain>.<subject>.<outcome>`。
- 消息用**简单英文短句**（见 `AGENTS.md` 的 Logging 段）：`Could not save session`、
  `Permission check denied`。禁止长难词与中文混排。
- 一个结果只在一个拥有该结果的层记录一次。
- 安全事实写 `audit_events`，日志只做诊断；二者不互相替代。

## 6. 死代码与可维护性

- 没有调用方的函数、类型、常量立即删除，不"留着以后用"。
- 重复实现同一语义的辅助函数（例如两个读内置角色 ID 的函数）合并为一个。
- 提交前运行 `gofmt`、`go vet`、`make test`；新增行为先写失败测试。
- 一个函数只做一件事；超过约 60 行或嵌套超过 3 层时拆分。

## 7. 并发、性能与可靠性

- 连接池每模块独立、有上限；慢查询必须有索引与超时。
- 事务内短小、固定加锁顺序；跨行不变量用 advisory lock 或唯一索引保证。
- 后台任务（清理、同步）必须可重入、可中断、分批处理。
- 外部调用设置超时；重试只用于幂等操作，并带退避上限。

## 8. 新增模块检查清单

- [ ] `src/apps/<module>/{cmd,internal,migrations,config.yaml,README.md}` 齐备
- [ ] 路由在 `internal/router`，每条路由声明认证方式
- [ ] 分层与依赖方向符合第 2 节
- [ ] 表结构与迁移符合[数据库设计规范](database-design.md)
- [ ] 关键日志事件与审计动作齐全，消息是简单英文
- [ ] 无死代码、无重复实现、无手改生成文件
- [ ] `make test`、`make build` 通过；集成测试使用自有资源

## 依据

- [Organizing a Go module（Go 官方）](https://go.dev/doc/modules/layout)：`cmd`/`internal` 的官方解释。
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout)：`internal` 由编译器强制私有、
  `pkg` 仅用于确实可复用的代码；并明确该布局不是官方标准，需按项目规模裁剪。
- [Effective Go — Names](https://go.dev/doc/effective_go#names)：命名长度与作用域的关系。
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)：错误包装、接口位置、包命名。
- [Kubernetes logging conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md)：
  结构化日志、消息风格、库不自行记录错误。
- [Prometheus](https://github.com/prometheus/prometheus/tree/main/cmd)、[Grafana](https://github.com/grafana/grafana/tree/main/pkg)、
  [Thanos](https://github.com/thanos-io/thanos/tree/main/cmd)：`cmd/` + `internal/`/`pkg/` 的真实大型布局。
