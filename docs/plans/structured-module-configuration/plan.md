# 实施计划

> 后续实现说明（2026-10-29）：本页保留历史方案与验收记录。HTTP/认证编排、配置加载和生成客户端的当前实现由 [Gin 全栈重构](../gin-service-refactor/acceptance.md)承接；历史 Authboss/Chi/环境占位方案不再约束当前实现。

> 密码行为以 [简化密码策略](../simplify-password-policy/spec.md) 为准；本文的其他功能和验证范围保持不变。
> Web 基准更新：本文的 Arco/Vite/Redux 前端实现记录已由 [Web 整体替换计划](../replace-web-template/plan.md) 承接。现采用 Next.js/shadcn 基准并重建 IAM 页面；服务端业务与授权契约继续有效。


状态：统一连接修订及本地验收完成；2026-10-28。未提交或推送。

依据：[Kratos配置](https://github.com/go-kratos/kratos-layout/blob/main/configs/config.yaml)、[环境占位解析](https://github.com/go-kratos/kratos/blob/main/config/options.go)、[go-zero配置加载](https://github.com/zeromicro/go-zero/blob/master/core/conf/config.go)、[Vite环境变量](https://github.com/vitejs/vite/blob/main/docs/guide/env-and-mode.md)、[Lumberjack](https://github.com/natefinch/lumberjack/blob/v2.2.1/README.md)。Go无统一强制文件名，本项目统一config.yaml。

- [x] 以配置契约测试确认结构化加载、环境默认值、空值、类型、安全解析与未知字段。
- [x] 修改IAM/Console配置和调用者；移除实例参数和文件型连接/敏感信息，维护迁移凭据隔离。
- [x] 采用Go标准slog及Lumberjack实现共享日志输出与轮转，增加有价值的日志契约测试。
- [x] 根脚本、Web示例、Compose、镜像与现有测试全部同步；开发构建使用系统临时目录并清理。
- [x] 迁移本机已生成的配置/证书，保留既有密码、密钥或凭证的值但不打印，删除自有.local内容。
- [x] 更新根/模块README、配置指南与计划索引；单元、真实集成、浏览器E2E、镜像及Compose核验。


## 原方案验收记录（统一连接修订前）

- `pnpm test:unit`通过：14个启动脚本测试、Go包测试及6个前端单元用例。新增配置用例验证嵌套字段、环境默认值/显式空值、密码保真与转义、未知/重复键及值不成为YAML结构；日志用例验证JSON、级别、实际文件轮转和非法配置拒绝。
- `node tests/integration/run.mjs --docker --race`通过，真实PG/Redis身份/会话/组织授权及恢复流程不变。
- `node tests/e2e/run.mjs --docker`通过：6个Chromium流程；两个IAM、两个Console分别使用日志文件，验证每条JSON的service字段及密码/token/密钥不出现在日志中。运行角色预建后由真实迁移命令授权，没有并行测试特权配置。
- `pnpm contracts:check`通过（40方法、15权限），类型检查与`make build`通过。
- 所有Compose镜像构建通过；隔离端口/空卷启动整套服务，验证真实登录/主动改密、内部接口隔离、运行账号最小权限及容器日志驱动轮转。自有容器及测试卷清理完毕。
- Go漏洞扫描0个可达漏洞，保留原有1项未调用依赖路径告警；没有声称依赖全无告警。
- 本机原有生成密码/凭证迁入被忽略的.env，TLS材料移入certs；已删除自有.local目录。根.env、任意logs目录和certs目录均被Git忽略，.agents/.github保留。
- 当前工作目录决定相对文件路径：仓库内从根执行统一写logs；二进制部署固定WorkingDirectory或用显式路径；容器默认stdout，不依赖源码路径或运行模式。

## 取舍

配置文件和默认值只有各模块config.yaml一份；共享代码仅负责安全标量展开和类型化加载，不引入配置中心或大型框架。Go官方slog提供结构化日志，Lumberjack仅负责文件轮转。根启动用只打印监听地址的白名单参数读取Go解析结果，避免再写第二套YAML/环境解析器。

Web保留原生.env.example→.env，不使用Go配置加载器。数据库/Redis/认证敏感信息直接注入环境；TLS证书才保留文件。实例隔离使用部署项目名、PG/Redis连接等原生机制，不增加instance_id或替代它的自造标识。


## 统一连接实施修订

状态：实施及本地验收完成。此前关于迁移/维护独立账号的方案已被本次用户要求替换。

- [x] 全局PostgreSQL/Redis默认值、模块直接覆盖契约测试。
- [x] 移除独立账号/授权脚本及配置，迁移、维护、HTTP复用同一数据库连接。
- [x] 根配置按模块分区，端点从实际模块配置派生，保留独立/远程运行可选覆盖。
- [x] Redis key集中定义类型化模板及统一前缀。
- [x] 同步Compose、测试、本机.env与文档，验证完整启动及三层测试。

修订后重新通过单元、真实 PostgreSQL/Redis 集成（含 race）、6 项 Chromium E2E、类型检查及构建。隔离 Compose 空数据库验证迁移、共享账号、真实登录、自定义端口和内部接口隔离；测试容器和卷已清理。
