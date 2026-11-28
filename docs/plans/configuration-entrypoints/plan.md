# 配置入口一致性实施计划

本次落实已确认的配置审查整改，使用当前分支。

1. 写出配置优先级、凭证边界、监听、反代、归档和测试工具的失败用例。
2. 保留显式空值，删除管理员密码的公开默认值，将监听与调用地址恢复为独立字段。用同一份 IAM 配置运行 Compose 迁移与 IAM，并验证公开 Web 模板可进入镜像构建上下文。
3. 复用 Web 的 Next.js Route Handler，将 Ingest 上游移到运行时，并保留设备推送的认证与流式请求。
4. 补全 Ingest 离线预检和归档命令的根入口；复用实际 TLS 密钥校验和 ClickHouse URL 校验。
5. 统一测试配置来源；使用 goose 环境配置和独占的 sqlc 临时文件。
6. 对齐模板、私有文件与所有权文档，运行脚本、Go 和 Web 单测，按可用隔离资源验证 Compose、实际浏览器代理与归档回读。

依据为仓库现有的 IAM/Console 运行时反代和配置所有权，以及 [Next.js Route Handlers](https://nextjs.org/docs/app/getting-started/route-handlers)、[Docker Compose 环境变量插值](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/) 和 [goose v3.29.0 环境配置](https://github.com/pressly/goose/tree/v3.29.0#environment-variables)。

## 验证记录

- 根模板和本地配置的 70 个键、顺序和注释对齐。已有本地值保留，`make config` 通过四个模块的离线检查。
- 根脚本套件通过 52 项，Compose 配置及构建上下文检查通过 3 项，Web 配置和代理用例通过 8 项。Go 单测和 Web 类型检查通过。
- Compose 实际启动了无应用凭证的 PostgreSQL、Redis。当前源码的应用镜像分阶段构建后，完整 Compose 通过迁移、跨容器就绪、Web 管理员登录、Console 授权事件流以及缺少 Ingest 上游时的 `503` 检查。隔离验收容器和数据卷已清理。
- 隔离 PostgreSQL 实测 `make sqlc-vet` 从根 dotenv 取配置并准备所有查询。隔离 Kafka 与 ClickHouse 实测根归档入口迁移、结构检查、消费和按事件 ID 回读。Ingest 原生 HTTPS 以测试 CA 实测通过根启动器的就绪探测。
- 同一 Next.js 生产产物重启后连接不同端口的 Ingest 上游。完整 Web 生产构建成功，Chromium 的 IAM 会话和 Ingest 只读权限两项验收通过。
- 当前工作区的全量套件仍有范围外失败。Web 国际化测试缺少规则编辑文案的 `target` 插值，FortiGate 浏览器流程在规则编辑器的字段映射步骤被禁用，Go 集成的旧规则迁移用例失败。
