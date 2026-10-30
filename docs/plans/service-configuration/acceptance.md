# 服务配置收敛验收

日期：2026-10-30；settings-design；未暂存、提交、推送或部署

## 最终契约

- 根 .env.example 是整套应用的一键配置模板；实际凭据只放根 .env，进程环境优先。模块保留独立配置，根入口可选择单独模块并注入同名变量
- 应用只提供 HTTP，HTTPS/证书交给 Nginx 等部署入口；删除应用 scheme、ca_file、TLS 配置及专用证书工具/复制逻辑
- 连接位置用统一 HOST/PORT，监听用 BIND_HOST（及确有必要时的 BIND_PORT）；Web 端口独立。Web 与 Console 消费同一 IAM 位置，删除 BASE_URL/PROXY_TARGET 别名
- 模块 YAML 声明 `${VAR-default}` / `${VAR:-default}`，可直接看到变量、优先级与默认值；移除单独 Go 绑定文件。先由 YAML 解析结构，再通过成熟参数组件逐值展开，Viper/mapstructure 严格解码及模块校验保持
- 配置固定服务名无实际用途，删除；PostgreSQL TLS 参数统一 SSLMODE。Web 绝对元信息地址无实际需求，删除；IAM 安全来源统一 SPX_WEB_ORIGIN，本机按 Web 端口推导，外部 IP/域名/HTTPS 场景明确填写
- Web 的 .env.example 定义变量，不自我引用；根启动无需第二份 .env.local。独立启动用官方 @next/env 加载后调用原生 CLI，未增加自定义 Next 服务

## 验证

| 范围 | 实际结果 |
| --- | --- |
| 定向失败先行 | 旧 HTTP/TLS、连接与监听混用、公开端口耦合、代理旧变量及 YAML 声明/字面值测试出现预期失败，实施后通过 |
| 单元 | 根脚本 27、Go 全包通过；Web 16 文件/39 用例通过 |
| 声明式配置 | 模块/共享环境优先级、默认数值/时长、显式空值、引号/换行/冒号与 `${...}` 秘密字面值、命令替换拒绝、严格未知字段/重复键/多文档与安全错误全部通过 |
| 静态 | TypeScript、目录产物 4 份、Go vet（含 integration）、Go lint 0 issues、diff 空白通过；Web lint 无错误，原有表格嵌套条件 warning 保留 |
| 真实组件 | 远程 PG/Redis 全 IAM/Console integration，`-p 1 -race -count=1 -timeout 10m` 退出 0；共享 configuration/httpx/iamclient race 通过 |
| 全链路 | 最终独立双副本 HTTP Preview 的 Chromium 11/11（3.2m）通过，覆盖组织隔离、CSRF、权限、会话、SSE 撤权及管理交互 |
| Web 单独运行 | 仅选择 Web，在配置端口真实启动，访问已有第二 IAM/Console 副本；context 成功、未认证业务请求401；不启动或迁移本地后端 |
| Chrome | 真实登录、Dashboard、Profile/Account、成员及当前组织；语言切换刷新保持登录、退出刷新仍回登录页。退出前 console error/warn 为0；关闭自有标签 |
| 正常开发 | 根一键启动成功，Web 5173 与 IAM HTTP ready 均200，保持运行 |

## 修正与边界

Web 启动初次因 @next/env 的 CommonJS 导出与原生 ESM 命名导入不兼容而提前退出；复现后改为组件实际支持的默认导入，真实 CLI、最终 Preview 和浏览器流程均通过。严格配置不再允许代理测试依赖隐藏默认地址，测试补齐明确环境。声明式转换时误删数据库 Name 字段的编译错误已修正，并在最终 Go 全包及真实回归中验证。

环境展开使用现成 POSIX 参数组件，不启动 Shell或配置命令执行器。先解析再逐值展开，避免环境值改写 YAML 结构；环境值不递归解释。没有增加另一套默认值或保留旧变量兼容分支。

测试仅使用远程组件的随机自有 PG 库及授权 Redis DB0 的精确自有键，不执行 FLUSH/前缀清理，不删除共享来源桶。Web-only 临时进程正常退出；独立 Preview 正常关闭 SSE 并精确清理临时资源，日志敏感值排除检查通过。正常开发库只做正常启动，不用于测试数据。

浏览器未读取原始 Network/CDP、Cookie、请求体或头。本轮没有部署 Nginx、验证公网 HTTPS 或构建容器；已有静态部署配置仅做契约同步。未进行外部 CodeRabbit 审查。
