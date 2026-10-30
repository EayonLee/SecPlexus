# 服务配置职责与统一来源

状态：用户要求按清晰、准确、标准的方式统一整改，当前功能分支实施及验收完成，未提交

## 决策

- 后端 IAM、Console 和 Next 原生服务统一 HTTP；删除 IAM 独有的原生 TLS 服务端证书及监听协议开关，HTTPS 由部署入口/反向代理终止。调用端采用标准 HTTP 传输与系统信任，不在模块维护 CA 文件
- 按用户补充要求，Go YAML 直接使用标准 `${VAR-default}` / `${VAR:-default}` 参数表达式，表达模块环境 > 共享环境 > 默认值。使用成熟 mvdan 参数组件，不手写解析器；先解析 YAML 再逐值展开，保持秘密字面值与类型边界；删除单独 Go 绑定文件，Viper/mapstructure 保留严格解码
- 监听 host 默认 0.0.0.0，独立命名 BIND_HOST；服务连接位置使用 HOST/PORT，可为 IP 或内网域名；不能将通配监听地址当作连接位置。可选 BIND_PORT 用于代理入口端口与本地监听端口不同的实际部署
- 根 .env 统一声明 SPX_WEB_PORT、SPX_IAM_HOST/PORT、SPX_CONSOLE_HOST/PORT；应用间协议固定 HTTP，外部 HTTPS 由反向代理提供。Console 及 Web 消费同一 IAM 连接位置，不分别维护 BASE_URL/PROXY_TARGET 别名。调用模块可显式覆盖实际不同的目标，模块独立运行继续使用进程环境及自己的配置
- 浏览器可信来源 SPX_WEB_ORIGIN 独立于监听端口，本机默认由 Web 端口派生；服务器 IP、域名和反向代理场景明确填写实际入口。保留来源/CSRF/Cookie 校验，不能从任意请求 Host/转发头推断信任来源
- Web 删除重复的绝对元信息地址及其配置，认证可信来源仅由 IAM 使用。非公开服务位置不放进 NEXT_PUBLIC。Web 不再在 TS 代码中写 IAM/Console 默认地址，独立运行由原生 env 文件提供
- Web 启动使用 Next 官方 @next/env 加载环境后调用原生 CLI，使 SPX_WEB_PORT 在根启动和独立启动一致生效；不新增自定义 Next 服务
- 根入口默认启动本地全部模块，支持明确选择 web/iam/console（如 pnpm dev web 连接远程服务）；不根据 IP 自动猜测部署拓扑，不自动远程启动进程。只迁移/启动明确选择的模块
- 构建无需私有 .env，使用根示例的非秘密配置作为后备，不创建或改写私有文件；Web 仍接收最小必要环境，不获得数据库、Redis、管理员密码或服务凭证

## 验收

YAML 声明式环境覆盖与 YAML 后备、监听/连接名称、根配置向 Web/Console 的共同目标、跨机器 IP/域名及外部 HTTPS 入口、公开 HTTPS 与后端 HTTP、独立 Web 端口、选择性启动及秘密过滤均有对应测试。远程 PG/Redis 自有资源 race 集成、全套浏览器 E2E、必要的真实浏览器验收。不部署、不构建容器、不提交。

参考：Viper 官方环境变量与优先级文档；仓库已安装 Next.js 的环境变量/@next/env、CLI 与 Metadata 文档。

补充审查：固定服务名称不再配置；POSTGRES_SSLMODE 与驱动语义对齐；全部现行代码、模板、文档和测试使用新名称，不保留旧变量兼容层。
