# 工程规范固化：数据库设计、后端结构与日志

## 状态

已实施（Windows 实测；规范文档已落地并回填 IAM，后续模块直接遵循）。

## 问题

1. 日志消息中英混排，且用词偏书面，英文不好的人难以见名知意。
2. 9 张表只有少数对象注释，字段含义、单位与约束没有沉淀在数据库里。
3. 数据库中 `status`/`state`/`actor_type`/`outcome` 等封闭取值在 Go 中散落为字符串字面量，
   新增或改名取值需要全仓库搜索，容易漏改。
4. 目录划分、分层依赖、命名、死代码处理没有统一规则，只在 IAM 里局部成立；新模块会各写一套。

## 行为契约

- 日志消息一律简单英文短句；事件名与字段保持 `<domain>.<subject>.<outcome>`。
- 每个业务表与每个业务列都有 `COMMENT ON`，由前向迁移维护。
- 封闭取值的唯一来源是 `internal/enum` 的类型化常量 + DDL `CHECK`；Go 比较与赋值不写字面量；
  HTTP `oneof` 标签由测试断言与常量集合一致。
- 结构、分层、命名、死代码与并发要求由 `docs/development/backend-structure.md` 定义，
  数据与迁移要求由 `docs/development/database-design.md` 定义，`AGENTS.md` 只做索引与少量硬性规则。
- 任何规范或修复的采用都需要外部证据（官方文档或主流仓库实践），并在提交说明中给出。

## 非目标

- 不改业务契约、接口路径、结果码语义与前端行为。
- 不引入新框架、不替换 Gin/GORM/Goose。（其中 GORM 一条已被 [sqlc-data-layer](../sqlc-data-layer/spec.md) 取代：数据访问改为 sqlc，GORM 已移除；Gin 与 Goose 不变。）
- 不做与本规范无关的重构。

## 验收

- `make test` 全绿（脚本、Go、Web）。
- `make build` 成功。
- 开发库迁移推进到含注释的版本，`iam` schema 内业务表与列注释覆盖 100%。
- `make dev` 启动后登录成功，服务日志为英文短句。
