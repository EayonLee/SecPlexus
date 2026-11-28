# Web 表面与国际化实施方案

以 `next-shadcn-admin-dashboard` 的根 `globals.css`、Dashboard layout、共享 Sidebar、Card、Dialog、Sheet 和 Popover 为参考。模板由根令牌定义画布、抬升内容和浮层，组件按语义取色。保留 SecPlexus 的业务和品牌蓝，按 [DESIGN.md](../../../DESIGN.md#secplexus-implementation-mapping) 统一浅色层级。

1. 合并系统设置与控制台的壳层间距。把工作台、接入、解析、审计、用户和角色列表接到同一个 `ListSurface`，详情及表单工作区使用共享 Card。浮层、原生控件和通知读取同一主题。
2. 按 next-intl 的请求配置和 `NextIntlClientProvider` 合并服务端与客户端的消息、时区和回退。为缺词时的筛选器和诊断补回归。权限目录、详情和编辑勾选共用双语权限说明。
3. 执行词典对齐、类型检查、生产构建和隔离 Playwright。对照模板和控制台的计算色值，浏览器验证主题、视口和浮层。用语言切换后跨路由流程识别可见键和 next-intl 错误。

依赖与存储保持现有位置。本次改动只消费已加载词典及当前页响应，没有新增 API 或独立查询。旧客户端或局部词典缺失时，共享控件使用已知标签，诊断仍由 next-intl 报告。

Docker 浏览器测试先通过 Kafka 协议检查就绪，再显式创建测试主题。外部模式仍只清理本轮创建的主题，Docker 模式随独占容器回收。设备推送回归覆盖浏览器创建接入点、真实 HTTP 接收和 Kafka 输出。

生产构建、类型检查和 108 项单测通过。隔离 Chromium 的 8 项业务回归覆盖语言切换、跨页导航、筛选和浮层、IAM 管理、展示时区与设备推送。另在 1440px 和 390px 交叉测量 9 个路由的明暗画布、内容和侧栏色值，并打开选择菜单、资料和用户弹框、角色抽屉、提示和危险操作确认框检查其表面。Impeccable 扫描无发现。
