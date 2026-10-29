# IAM与设置区实施计划

> 后续实现说明（2026-10-29）：本页保留历史方案与验收记录。HTTP/认证编排、配置加载和生成客户端的当前实现由 [Gin 全栈重构](../gin-service-refactor/acceptance.md)承接；历史 Authboss/Chi/环境占位方案不再约束当前实现。

> 密码行为以 [简化密码策略](../simplify-password-policy/spec.md) 为准；本文的其他功能和验证范围保持不变。
> Web 基准更新：本文的 Arco/Vite/Redux 前端实现记录已由 [Web 整体替换计划](../replace-web-template/plan.md) 承接。现采用 Next.js/shadcn 基准并重建 IAM 页面；服务端业务与授权契约继续有效。


> 数据库账号方案已由[结构化模块配置修订](../structured-module-configuration/spec.md)替代：运行、迁移和维护默认共用同一账号；下文对应记录保留原方案验收历史。

状态：首版实施与本地验收完成；2026-10-28。配置/日志实现已由[结构化模块配置计划](../structured-module-configuration/plan.md)修订。IAM、Console与Web已接入真实身份链路，单元、真实组件集成及浏览器E2E已执行。未提交或推送。

[需求规格](spec.md) · [长期业务/技术文档](../../../src/apps/iam/README.md)

## 1. 当前结论与审计纠正

用户要求“没有安装流程、基础身份清晰”，同时明确首版必须支持组织内自定义角色、权限编辑和多角色组合。不能把精简误解为删掉可扩展授权，也不能用“未来可扩展”代替实际能编辑的角色管理功能。

本版保留完整的组织RBAC关系模型、统一授权入口及资源范围契约，取消没有需求支撑的安装和身份产品扩张。未实现的后续资源只定义接入边界，不先建表。

| 发现 | 原方案问题 | 本次处理 |
|---|---|---|
| 安装流程过重 | installation表、安装能力、setup API/UI、额外CLI生命周期都没有当前需求 | 部署迁移+幂等种子自动建立默认组织和首位超级管理员；直接登录 |
| 身份与业务角色混淆 | Owner、Analyst、Viewer与管理员并列；中途又误收敛为只能三角色 | 超级管理员为全局受保护身份；组织内系统org_admin/member；审计员等由用户自建，不按名字授权 |
| 扩展功能绑死首版 | 邮件邀请/Outbox、MFA、SSO、机器账号、配置中心都列成完整发布门槛 | 从首版表、API、配置和验收移除；以后有实际需求再设计 |
| 会话适配过度自研 | SCS之上再做CtxStore、generation及活动识别，增加维护面 | 官方PG Store+可撤销逻辑Session；旧存储写回不能恢复逻辑身份；先实验验证库接口 |
| 前端重复状态/权限 | 拟强加Query库；当前模板按菜单key和localStorage角色生成权限 | 先复用Axios/Redux；统一能力投影、页面清单和守卫，移除真实路径的演示提权 |
| 设置区语义不清 | 齿轮仍是模板页面配置；个人设置混管理入口 | 齿轮提供单一管理中心入口；内部组织/平台/系统设置分区，头像放本人事项；设置区保留AppShell并切换侧栏 |
| 未来数据分权易漏 | 单个布尔授权可能被当作全部资源访问；菜单权限易与API混淆 | 明确capability→resource scope→业务执行三个检查层，统一契约、查询前过滤及未知scope拒绝 |
| 配置规则过早固定DB | 会话时长被要求放数据库，与尚不存在的配置UI/策略表绑定 | 首版类型化部署配置；真实需要UI编辑时再迁移，不能环境/数据库双真源 |

Spec管需求验收；README管模块契约；本Plan管任务与证据。2026-10-28补充的配置/启动及测试规则见[配置指南](../../configuration.md)与[测试指南](../../testing.md)；不把用户举例视作新的业务范围。

## 2. 公开技术参考

参考资料核对日期为2026-10-27至28。以下公开资料用于解释架构取舍，不代表本项目已经实现或验证对应能力。

| 公开资料 | 相关设计 | 本项目取舍 |
|---|---|---|
| [Superset SecurityManager](https://github.com/apache/superset/blob/master/superset/security/manager.py) | get_user_roles、can_access、权限视图关系及对象访问检查 | 参考权限集合/集中检查；不把页面权限当数据范围，角色体系按本项目需求定义 |
| [Rancher授权类型](https://github.com/rancher/rancher/blob/main/pkg/apis/management.cattle.io/v3/authz_types.go) | GlobalRole、RoleTemplate、规则与Builtin字段；global/project/cluster范围区别 | 参考基础身份/可编辑规则/范围的分离；不引入Kubernetes RBAC控制器或角色继承链 |
| [Gitea Team](https://github.com/go-gitea/gitea/blob/main/models/organization/team.go) | OrgID、TeamUnit和repo范围组合 | 参考组织/成员集合/资源权限不同实体；不为未来用户组先建角色树 |
| [Grafana导航](https://github.com/grafana/grafana/blob/5d4a6f3bd2ed02b4e232770d5b29af2141d8da08/pkg/services/navtree/models.go) | RemoveEmptyAdminSections等，子入口过滤后再裁空管理分组 | 设置入口随可见子页派生，不留空管理菜单 |
| [Langfuse routes](https://github.com/langfuse/langfuse/blob/d89f49b82620dc9a773bd2f97449289f4f6c199e/web/src/components/layouts/routes.tsx)、[navigationFilters](https://github.com/langfuse/langfuse/blob/d89f49b82620dc9a773bd2f97449289f4f6c199e/web/src/components/layouts/app-layout/utils/navigationFilters.ts) | organization/project上下文、导航定义及RBAC/功能等分维度过滤 | 参考作用域导航和集中裁剪；不引入付费能力、项目层级或商业entitlement体系 |
| [Open WebUI access_control](https://github.com/open-webui/open-webui/blob/33c1978d5bad52a31df75ea55143025dfad18d16/backend/open_webui/utils/access_control/__init__.py) | combine_permissions并集、资源sharing key及access grant校验 | 借鉴功能授权/共享资源访问分层；本项目统一校验数据范围并保持授权单一真源 |
| [Authboss](https://github.com/aarondl/authboss)、[SCS Store](https://github.com/alexedwards/scs/blob/master/store.go)、[SCS会话](https://github.com/alexedwards/scs/blob/master/data.go) | 可嵌入流程/存储接口；SCS Store键哈希、续期与Token轮换 | 复用而不重写协议；库默认行为需实验验证，不把Store残留行等同有效身份 |
| [Goose provider](https://pressly.github.io/goose/documentation/provider/)、[PG建库](https://www.postgresql.org/docs/current/sql-createdatabase.html) | 显式锁与版本迁移、建库非普通事务 | 建库部署负责，迁移任务取锁，运行角色最小权限 |

参考项目许可及付费能力不同；这是模式与代码研究，不代表其所有功能均为可免费嵌入依赖。实际依赖单独核验版本/许可。

## 3. 实现原则及数据真源

1. 领域用例处理业务不变量；Authboss/SCS是适配器，不让它们的User类型蔓延到Console。
2. 一个授权决策入口：已知action、组织、有效身份、当前DB角色grants、资源真实范围。所有通道共用同一规则，UI只是能力投影。
3. 用户自定义的是角色组合，不是权限语言；系统定义action语义。按名称判断“安全分析师”属于禁止模式。
4. 超级管理员标识受保护；org_admin/member是不可编辑系统角色；普通角色可以编辑、分配和停用，多角色并集，无静态兜底。
5. 功能授权、数据分权、执行审批拆清责任；不把组织边界误当细粒度对象ACL已经完成。
6. 当前没有实际需求的引擎/框架不安装；后续求值可替换而路由契约不变，不叠加第二份Casbin/OpenFGA策略真源。

业务及库适配表为README §4的11张：users、password_credentials、organizations、memberships、roles、permissions、role_permissions、membership_roles、user_sessions、scs_sessions、audit_events。保持完整FK/唯一键/类型，生成SQL前核对每个复合FK的目标唯一约束。没有真实用户历史，不将Mock localStorage角色迁为真实管理员。

版本：Authboss 3.5.3、SCS 2.9.0、pgx 5.11.0、Goose 3.24.3已锁定；sqlc 1.29.0和代码生成器固定版本。Go工具链统一为1.27.3，go.mod、CI、镜像同步；不修改开发者全局Go安装。前端保留React/Arco/Vite/Axios/Redux，不新增第二套状态库。

## 4. 分阶段实施

以下任务按当前首版范围完成。Console目前只有组织上下文及SSE；不存在的业务导出、批量、任务执行和对象ACL不创建占位实现。相关条目指已存在接口的边界与以后接入约束，详见下方验收说明。

### P0：先证明契约与库适配

- [x] 固定依赖、Go兼容、PG/Redis镜像及CSRF中间件版本；核对Authboss所需User/Store/SessionState接口，关闭本版未交付模块。
- [x] 写公开/内部OpenAPI及权限清单，生成Go/TS类型；前缀与路由action注册完整性CI校验。
- [x] 定义Principal、CapabilityDecision、AuthorizedScope、业务ResourceDecision的窄类型；v1只有user主体；platform管理范围与organization业务范围条件明确，未知类型明确拒绝。
- [x] 验证Authboss仅启用auth的JSON响应、SessionState Put/Del/DelAll、SCS PG Store可配置表名/键哈希、Load/Commit与成功响应顺序；本人身份边界不能绕过。
- [x] 实验官方Store+逻辑Session：登录/重新认证轮换新逻辑ID，旧SCS延迟写回不能恢复身份；不提前重写CtxStore。
- [x] 并发重置密码/禁用期间旧密码请求建会话失败；SCS RenewToken不延长逻辑absolute；逻辑Session条件UPDATE维护idle/absolute，SCS IdleTimeout=0；旧请求跨idle后保存、两副本乱序保存不能重新激活/倒退逻辑期限；Console内部authorize续活不依赖Set-Cookie；绝对期限有效。
- [x] 验证SCS实际保存失败不发成功Cookie，补偿逻辑行；统计写入不改变撤销状态。

通过：AC-05/06/07核心场景。无法证明适配时记录具体接口限制，调整设计，不私自换独立IdP或补一套JWT。

### P1：自动初始化及真实登录

- [x] 应用只按配置端点连接基础组件，make dev不调用Docker；Compose作为独立部署入口提供全部初始基础组件。启动统一检查.env，缺失时从示例生成并要求检查，不覆盖已有文件。后端实现后接入迁移、种子及Go/Web热重载；Make仍只有dev/build/stop。
- [x] IAM正式迁移和命名查询，受控迁移锁、运行角色权限、schema兼容检查；权限清单及系统角色种子幂等。
- [x] 空账号库从Secret读取初始密码，事务创建default组织/admin/受保护org_admin及member角色/绑定/审计；非空库不重置密码，不提升普通账号。
- [x] Argon2id Hasher、登录限流/短期锁定、Cookie/CSRF、me/改密/Session设备管理；多副本共享PG。
- [x] Web真实认证状态和账号安全页；移除admin/admin预填及认证Mock真实路径；本人改密后重新登录。
- [x] Console首个组织API通过内部TLS/服务凭证调用IAM；失败401/403/503分开，不能fallback放行。

通过：AC-01/02/05/06/07/09/10。验证空启动、重复/并发启动、缺Secret、迁移/PG/Redis故障、两个副本撤销、真实浏览器链路。

### P2：自定义角色和组织授权闭环

- [x] 用户/组织/成员CRUD与账号状态；新组织同事务创建本版系统角色及显式grants并绑定管理员；所有管理写用例统一事务锁、当前授权复核和最后管理员保护。
- [x] 角色CRUD、完整权限替换、成员多角色绑定、停用/删除影响检查、预览；系统角色保护，平台权限不能写入组织grants。
- [x] org_admin不能授出超级管理员/org_admin；普通委派管理员不能授出自己没有或不可委派的权限，旧/新角色权限均校验。
- [x] role.version与org.authz_version乐观锁；角色/权限/成员/审计同事务；并发撤权/转授重读当前状态，不信任请求开始时缓存。
- [x] IAM统一Resolve/Authorize；Console两个现有路由服务端确定action及org_id并验证返回scope；IAM成员/角色/审计查询及分页count显式限定组织。Console无业务仓储、聚合、批量或导出接口，本版不创建占位。
- [x] 有效权限查询、候选修改预览、目录分组元数据；无角色授权为空，角色改名与显示语言不改变权限，新增目录项不授予已有自定义角色。

通过：AC-03/04/08/13/14。测试自建审计员只读、分析师选定动作、多角色并集、清空授权、跨组织绑定、改名、删除在用角色、最后管理员和并发冲突；不能仅测权限编辑器写库成功。

### P3：设置区与业务授权接入

- [x] 以当前NavBar/layout/routes/PermissionWrapper为起点，先迁移能力模型，不直接在每个组件加isAdmin判断。
- [x] 一份PageDefinition声明作用域/layout/入口/action要求，派生管理中心入口、侧栏、守卫、面包屑、默认落点；空分组裁剪。
- [x] 齿轮→单一管理中心，内分组织与访问/平台管理/系统设置；头像→本人资料/账号安全/退出；移除角色切换演示；不改Arco主题基线。
- [x] SettingsLayout复用AppShell顶栏和左侧栏，切菜单树；提供返回业务区与明确当前组织；角色编辑页内Tab组织信息/权限/成员。
- [x] 页面依据后端capabilities；对象按钮依据allowed_actions；useCan/Can/RequireAccess统一调用权限投影。全局平台能力也由后端返回，前端不自己扩展超级权限。
- [x] 认证与安全页首版只显示脱敏概览，不展示未实现的MFA/SSO配置；GET安全概览端点只返回白名单字段。
- [x] 只读角色管理员/审计员按对应action看到子页面，不要求持系统org_admin才能进入整个设置区；入口跳首个可访问子页。
- [x] 组织切换/跨标签退出/迟到请求/权限被撤销统一处理；无权深链403，503故障不伪装退出；密码留在头像安全页。

通过：AC-09/15。真实浏览器至少走超级管理员、org_admin、普通无业务权限成员、自定义审计员、自定义角色管理员五种权限组合；这里是测试组合，不新增内置角色。

### P4：全链路和维护验证

- [x] 真实PG/Redis可使用隔离容器或受控测试环境；两个IAM/Console副本，所有首版AC通过；验证管理入口；CLI维护使用独立部署凭证与相同密码/会话不变量。当前没有业务后台任务。
- [x] SSE初始及每25秒重新授权（单次IAM请求最多5秒），返回并记录用户与decision_id；所有IAM高危管理写用例在事务内复核。未来业务执行点再次授权，当前不建立任务执行系统。
- [x] 现有SQL/范围契约验证：列表、count和单项查询均限定组织，未知scope拒绝；本版没有导出/批量业务路由，不声称对象ACL已实现。
- [x] 迁移锁/失败恢复、schema版本不兼容时拒绝启动；真实PG备份恢复后撤销Session再开放流量。不自动破坏性down/清库；当前只有首个schema版本，不声称历史版本滚动升级已验证。
- [x] make dev/build/stop、Secret注入、日志脱敏、监控与审计维护任务；已登录权限撤销不存在TTL允许窗口。
- [x] 分页、有界请求、固定权限查询与并发密码哈希上限已落地；无权限正缓存。容量与性能SLA未做负载测试，不据本地功能测试给出吞吐达标结论。
- [x] 根据真实接口/目录刷新README的目标/已实现状态，Plan记录验证证据与未完成项。

## 5. 扩展约束，不扩张当前任务

Agent、Skill、组织记忆等仅是用户对未来接入的举例，不建立对应功能、表、页面或排期。当前落实统一Principal/Authorize/Scope契约，功能与对象范围明确分层，所有数据入口应用真实范围，查询/检索前过滤，服务执行保留原始主体。

新业务按“权限清单+路由/用例声明+资源范围适配”接入；用户组、对象共享/ACL、字段权限有明确业务时再设计局部数据模型。不得把举例变成首版需求，也不承诺未知业务永远零改造。设计模式只用于减少耦合、统一不变量或隔离变化，不能为使用模式而造框架。

## 6. 验证入口与证据

| 层次 | 计划入口/环境 | 验证重点 |
|---|---|---|
| Go领域/适配 | go test ./src/apps/iam/...；关键并发包-race | 委派上限、系统角色保护、期限/版本/状态机、库响应顺序、未知scope |
| 真实PG/Redis集成 | tests/integration/run.mjs；独立测试库/端口 | FK/锁/事务、角色清空、Token残留、并发权限编辑/改密/管理员保护，不碰用户正常库 |
| 契约 | OpenAPI/权限清单lint与生成diff | action语义、API前缀、字段、Principal/Scope，路由及页面引用key有效 |
| 浏览器 | 真实Web→Console→IAM | 设置导航、个人安全、五种权限组合、角色编辑即时生效、401/403/503、缓存隔离 |
| 迁移与恢复 | 空库/重复启动/N→N+1/隔离恢复 | 自动初始化、不持有DDL、初始化不覆盖已有凭据、恢复后旧会话失效 |

测试用例归属、隔离及临时排障边界见[测试指南](../../testing.md)。开发脚本单元测试不单独代表身份功能验收；已执行的浏览器E2E经过真实Web→Console→IAM和真实数据存储，不使用模板角色切换或Mock登录冒充。

## 7. 实际交付与验证证据

本地核验日期：2026-10-28。记录的是已执行结果，远端CI尚未运行。

| 阶段 | 实现入口 | 验证结果 |
|---|---|---|
| P0 契约/适配 | contracts/iam、contracts/console；Authboss/SCS适配 | 40个API方法、15个action检查通过；Go/TS生成及sqlc可重复生成；库会话桥接、成功响应顺序、旧字节写回及并发重新认证验证通过 |
| P1 初始化/登录 | IAM CLI、嵌入迁移、种子、身份页面 | PG/Redis真实集成通过；默认组织与管理员、幂等/并发初始化、主动改密、两副本撤销、依赖故障及SCS失败无成功Cookie |
| P2 组织/自定义授权 | application管理事务、roles/preview、权限目录 | 多角色并集、空授权、系统角色保护、委派边界、版本冲突、审计失败回滚、最后管理员并发保护通过；预览覆盖有效状态和废弃权限的新旧绑定边界 |
| P3 管理中心 | Web pages/management、auth、account | 浏览器验证超级管理员、组织管理员、普通成员、自定义审计员、自定义角色管理员；角色创建/权限编辑/预览、深链拒绝、跨标签退出与组织上下文隔离通过 |
| P4 维护/交付 | 启停脚本、Compose、CLI维护、测试及CI | 单元/集成-race/真实Chromium E2E通过；make build和镜像构建通过；独立Compose空卷启动及最小权限核验通过；临时资源清理完成 |

实际执行命令：

```sh
pnpm test:unit                         # 13个脚本用例、Go包测试、6个前端单元用例
pnpm contracts:generate
pnpm contracts:check                   # 40个API方法，15个权限动作
pnpm typecheck
make build
pnpm test:integration --docker --race
pnpm test:e2e --docker                 # 6个真实Chromium核心流程，两个IAM/两个Console
GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...
```

Go漏洞扫描报告0个可达漏洞；另有1项依赖告警涉及未调用路径，不把它写成依赖完全无告警。
授权单元测试另经受控条件反转验证：主动改密条件被反转时测试失败，随后恢复原实现。
浏览器截图人工检查过管理中心与角色权限编辑器，保留Arco组件和主题基线。

### 验收对应关系

| AC | 证据 |
|---|---|
| 01–02 初始化/最小权限 | session、migration集成测试；独立Compose全服务空卷启动、真实登录及运行角色权限检查；脚本stop只停止自有监听进程，保留外部进程 |
| 03–04 基础身份/保护 | 纯委派规则、真实自定义角色生命周期、多角色并集和最后管理员并发用例；五种权限组合浏览器导航 |
| 05–07 登录/会话 | Authboss HTTP生命周期、锁定恢复、密码版本检查、idle/absolute受控DB时钟、跨副本旧SCS写回、跨账号轮换及并发重新认证 |
| 08 跨边界 | 公开及内部写请求CSRF、原Cookie+TLS服务凭证、限定组织SQL、跨组织角色绑定拒绝、非法返回地址拒绝、未知scope及非空对象上下文拒绝；当前无业务批量端点，不生成假接口 |
| 09–11 Web/故障/配置 | 真实Web→Console→IAM、跨标签退出/组织切换、迟到请求与503前端单元；PG/Redis故障及审计失败集成；类型化配置与环境占位单元 |
| 12 维护/长连接 | pg_dump→pg_restore→恢复后撤销旧会话真实集成；迁移失败重试及并发迁移；浏览器SSE撤权断流；高危IAM管理写事务复核 |
| 13–14 自定义授权 | 授权集成覆盖改名、空grants/绑定、并集、委派撤权、在用角色删除409、角色及组织版本冲突和有效权限预览 |
| 15–16 UI/契约 | PageDefinition派生导航/守卫、五种组合/无权深链；OpenAPI校验与生成、权限常量、40路由检查；安全概览白名单 |

### 当前边界

- 当前只有IAM权限动作和Console组织上下文/SSE，未来告警、Agent、Skill、工作流、导出及高危执行不在本版交付。
- v1仅组织范围，不含对象共享/ACL、字段级分权、用户组或角色继承；未知scope拒绝。
- 迁移目前为首个版本；已验证失败回滚及恢复，不声称验证过尚不存在的旧版应用升级。
- 无公开注册、邮件恢复、MFA、SSO或独立身份服务。模板示例可选开发模式保留，真实认证永不Mock。
- 尚未提交、推送或运行远端CI；本地测试不替代后续部署环境的容量与运维验收。
