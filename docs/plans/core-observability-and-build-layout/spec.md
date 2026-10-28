# 核心链路日志与构建目录整理

状态：按本次明确指令实施；属于既有工程规范的补足，不更换认证产品或 HTTP 契约。

## 行为与验收

- 沿用 slog 与现有轮转日志。共享 logging 提供请求上下文和安全错误诊断；调用点仍明确使用 Info/Warn/Error/Debug，不再装一套日志框架。
- 所有 Go HTTP 入口生成 request_id；记录方法、注册路由、状态、字节和耗时。未知路由不记录原始路径或查询参数；正常健康检查降到 Debug。panic 保留类型与代码位置，未提交时返回统一 Result，流已开始后不再改写 JSON。
- IAM 覆盖请求绑定/CSRF/服务认证拒绝、登录/密码确认、逻辑会话提交/补偿/撤销、授权、所有管理写事务、数据库错误/慢操作及 Redis 限流。Console 覆盖内部调用、组织范围校验和 SSE 开始/撤权/断开；Web 当前已替换为 Next 模板，使用 instrumentation 的 onRequestError 诊断现有服务端渲染/Action/路由错误，以 digest 关联框架错误，不打印 request/response 原对象；不恢复已删除的 Arco 认证客户端。Web 替换计划维护业务界面；本轮只补充其已经落地的 API 代理、客户端异常与请求标识，不重建页面。
- 管理事务以 action/org_id/actor_id/target/阶段说明结果，仅提交后记录成功；失败不能吞掉或继续成功。技术故障保存安全类别、SQLSTATE/约束、代码位置和耗时，禁止密码、Cookie、Token、DSN、SQL 参数、请求体或数据库错误 Detail 进入日志。业务拒绝按既有 code 返回；前端按 code/status 处理，message 不作为分支条件。
- 不打印每次成功 SQL、每个字段、心跳或循环；数据库异常/慢操作及 HTTP/业务结果各有职责，不逐层复制同一错误文本。日志是运行诊断，不替代数据库事务审计。
- Redis key 前缀固定 spx，删除配置、环境变量和 Format 前缀参数；保留账号/IP 摘要和 TTL。测试用例只清理自己创建的精确 key，不使用全局匹配删除。
- `deploy/docker` 改为 `deploy/docker`。当前 Go 模块共享一个带 APP 参数的 Dockerfile，Web 单独一个；Compose 及后续编排共用镜像定义，仓库根是 build context，不在模块或编排目录复制构建文件。本轮不创建 K8s。
- 你指出的角色分配等核心函数按校验/授权/修改/审计分段，中文注释说明锁、委派与事务约束；不靠逐行翻译或额外层次冒充可读性。同步 code-standard。

## 依据与取舍

Dockerfile 放置没有唯一标准：[Gitea](https://github.com/go-gitea/gitea/blob/main/Dockerfile) 放仓库根，[Grafana](https://github.com/grafana/grafana/blob/main/Dockerfile) 同时有根 Dockerfile 与 packaging/docker 辅助文件，[Harbor](https://github.com/goharbor/harbor/tree/main/make/photon) 集中维护多服务镜像。当前仓库单 Go module、共享 contracts/packages，集中到 deploy/docker 可避免多份构建逻辑漂移；模块根适合独立仓库/独立构建边界。Docker 的 COPY 路径由 [build context](https://docs.docker.com/build/concepts/context/) 决定，与 Dockerfile 存放位置不是同一概念。

日志依据 [Go slog](https://pkg.go.dev/log/slog) 的结构化与 Context API、[GORM Logger](https://gorm.io/docs/logger.html) 的上下文 Trace 接口；自定义适配只输出受控诊断，不输出 SQL 与参数。不引入日志平台、全量分布式追踪或前端遥测上传服务。
