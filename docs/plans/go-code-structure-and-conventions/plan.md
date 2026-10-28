# 实施计划

状态：实施及本地验收完成。

- [x] 参考公开 Go 编码规范、路由文档及现有项目实现，检查当前结构。
- [x] 整理 AGENTS 通用规范与项目约定。
- [x] 提取具名 HTTP 处理函数，集中路由注册；梳理格式和关键注释。
- [x] 运行契约、单元、集成、浏览器 E2E 和构建检查，并记录结果。

契约检查通过（40 条路由、15 项权限）；单元测试、真实 PostgreSQL/Redis 集成测试（含 race）、6 项 Chromium E2E、类型检查、构建和隔离 Compose 验证通过。

## 后续修订：中文注释与启动组装

- 集中 IAM 公开及内部接口和 Console 接口的路由注册，保留原中间件顺序。
- 把服务依赖组装从命令参数处理分离，明确初始化及关闭顺序；环境变量只由配置加载边界读取。
- 配置、日志等关键结构增加对齐的中文字段注释，并补充服务和用例的中文函数注释。
- 重新验证契约、单元、集成及浏览器流程。

本次修订验证：`pnpm test:unit`、`node tests/integration/run.mjs --docker --race`、`node tests/e2e/run.mjs --docker`（6 项 Chromium）、`pnpm contracts:check` 和 `make build` 通过；补充纯注释后再次运行 `go test ./src/...`。
