# IAM 统一入口验收

日期：2026-10-30；分支 settings-design；未暂存、提交或推送

## 结果

IAM 单监听，`POST /api/v1/iam/authorize` 与用户 API 使用同一前缀。删除旧路由、内部端口、internal 配置及强制 TLS；服务凭证收敛至 service_clients，用户与服务认证规则独立。默认 HTTP，成对配置 app TLS 证书和私钥时唯一监听使用 HTTPS。根启动按实际监听协议推导连接目标，HTTPS 就绪探测验证证书；Console 支持系统信任及显式 CA，不跳过验证。默认隔离浏览器栈不再生成证书或申请内部端口。同步现行文档、环境示例和已有 Compose 静态配置，未运行部署或容器构建。

## 实际验证

- 定向失败先行：原客户端拒绝 HTTP、服务认证强制 TLS、旧地址推导与不完整 TLS 配置测试失败；修改后通过
- 根脚本 27 用例、Go 全包与 Web 37 用例通过；类型检查、目录产物检查、Go vet（包含 integration 构建标签）、Go lint 0 issues、diff 空白检查通过；Web lint 无错误，保留原表格嵌套条件 warning
- 远程 PG/Redis 的完整 IAM/Console integration：`-p 1 -race -count=1 -timeout 10m` 退出 0；包含真实 HTTP/HTTPS 启动、同监听认证隔离、原会话/权限/CSRF/迁移及备份恢复
- 真实 HTTPS 客户端接受显式可信 CA，不可信证书拒绝；一次性隔离根 Preview 验证 HTTPS 监听发现、证书校验的就绪检查、Web HTTPS 上游及 Console HTTPS 鉴权，正常退出并清理临时证书
- 双副本 HTTP Preview 的 Chromium 全套 11/11（3.3m）通过，包括撤权 SSE、组织隔离、跨标签退出、成员选择及原弹框身份确认
- Chrome 专用代理验收真实登录、Dashboard、Settings Profile/Account、System 成员及当前组织、语言切换后刷新保持登录、退出后刷新回登录页；退出前普通 console error/warn 为 0；不声明人工访问过 HTTPS 页面
- 正常开发入口 Web 200、IAM HTTP ready 200；缺少服务凭证的 authorize 返回 403；旧路径返回 404；旧内部端口无监听

## 修正及边界

首轮真实组件检查发现测试缺少 strings 导入、备份恢复未使用匹配版本客户端，以及新增监听测试 fixture 未初始化 IAM；分别修正导入、使用匹配 PG 工具和正常 Seed 初始化。没有降低认证或测试断言、提高重试次数、改变共享资源规则。最终完整回归通过。

所有测试仅使用远程组件的随机自有 PG 库及随机账号/会话键；Redis DB0 已获授权，不执行 FLUSH、前缀清理或删除共享来源桶。隔离栈正常退出，Console SSE 观察取消及结束，结构化文件日志敏感值排除检查通过，临时数据库/目录已按所有者清理。浏览器未读取原始 Network/CDP、Cookie、请求体或头。正常开发服务已重启并保持运行；本地私有环境移除旧监听/TLS 必填项，未改 PG/Redis 连接或管理员凭据。

用户后续提出的统一配置来源、监听/连接/公开地址关系需要另行梳理；本验收只证明本计划的单入口及协议收敛，不声称整个配置模型已完成重设计。
