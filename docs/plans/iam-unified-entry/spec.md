# IAM 统一入口

状态：用户已在对话确认单端口、统一 API 前缀、HTTP 默认及可选 HTTPS，并授权在当前功能分支直接实施，不提交代码

## 行为契约

- IAM 单进程只创建一个 HTTP 监听；全部业务接口采用 `/api/v1/iam/...`，服务鉴权为 `POST /api/v1/iam/authorize`；删除旧监听和旧路由，不保留兼容入口
- 默认监听 HTTP，通过 IP 或域名访问均可；app.tls_cert_file 与 tls_key_file 成对配置时同一监听使用 HTTPS，不额外创建 HTTP 监听
- 浏览器公开 URL 独立控制 Cookie Secure 与 CSRF 来源；入口 HTTPS 可终止于反向代理，IAM 与 Console 仍可通过 HTTP 通信
- 登录/上下文/就绪检查保留既有公开规则；用户接口保留 Session、CSRF、权限、近期认证和组织隔离
- authorize 只接受配置服务凭证及允许的 action；服务凭证不能登录用户接口，用户 Cookie 不能替代服务凭证。原用户身份、有效会话、Origin/CSRF、组织与权限仍由 IAM 检查
- 服务鉴权接口不加载调用方浏览器 Session，也不设置用户 Cookie；不按 IP、Origin、TLS 状态或浏览器传入的 subject_id 免认证
- 服务凭证配置收敛为 service_clients；HTTP 客户端接受 HTTP(S) origin，HTTPS 使用系统信任或显式 CA，禁止跳过验证、自动降级或跨地址重定向
- 根启动从 IAM 实际监听信息推导 Console 与 Web 目标；删除内部端口、自动测试证书需求和默认证书路径。自定义端点仍可覆盖
- 无数据库迁移、无 UI 改版、无新认证栈、无部署/CI/容器工作

## 验收

配置默认 HTTP 无证书启动，缺半套 TLS 拒绝；HTTP 和有效 HTTPS 完成真实通信，不可信证书拒绝。统一入口服务凭证缺失/错误/action 越权/仅用户 Cookie 拒绝；有效服务+用户会话正常授权，无用户会话拒绝。用户接口权限与 CSRF、双副本会话撤销和 Console SSE 保持。根启动、文档、测试和项目规则无现行旧入口约定；隔离远程组件回归，不使用开发库测试

后续 [服务配置收敛](../service-configuration/spec.md) 进一步删除应用服务端 TLS 与 CA 配置，HTTPS 统一交由反向代理；以新规格为准，本文件保留原实施时的范围。
