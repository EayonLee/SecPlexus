# 角色管理：实际开发贯通范例

本页链接当前实现，不复制一套会过时的演示源码。核心规则是组织隔离、实时授权、委派上限、版本冲突、系统角色保护和变更/审计同事务。

| 环节 | 当前文件与职责 |
| --- | --- |
| 协议 | [原生路由](../../src/apps/iam/internal/router/routes.go)、[Go DTO](../../src/apps/iam/internal/model/http.go)、[权限目录](../../src/apps/iam/internal/authorization/permissions.json)：拥有方法、路径、输入、结果及授权；原生Go定义服务端行为，Web消费模型随调用方维护 |
| Gin接入 | [路由](../../src/apps/iam/internal/router/routes.go)、[管理Handler](../../src/apps/iam/internal/handler/management/roles.go)：原生 Gin 绑定 DTO；错误加入Gin错误链，成功输出Result/204 |
| 安全与身份 | [入口装配](../../src/apps/iam/internal/router/routes.go)：公开会话/CSRF/限流/协议与内部服务认证隔离；HTTP context只携带请求状态 |
| 应用用例 | [角色用例](../../src/apps/iam/internal/service/roles.go)、[管理事务](../../src/apps/iam/internal/service/service.go)：Transaction回调内重验身份、组织/用户锁、权限和版本，变更及审计一起提交 |
| 仓储 | [角色关联](../../src/apps/iam/internal/repository/roles.go)：读取旧授权一次、批量插入新增关系；保留未变更的弃用授权，避免重新触发新增约束；成员关系批量写入仍受组织复合FK约束 |
| 数据类型 | [普通列表](../../src/apps/iam/internal/repository/lists.go)、[查询](../../src/apps/iam/internal/repository/queries.go)：GORM泛型和具名公开字段投影；特殊锁/数组查询保留必要参数化SQL，不建设BaseRepository |
| 客户端 | [Axios边界](../../src/apps/web/src/spx/api/client.ts)、[会话](../../src/apps/web/src/spx/iam/session.ts)、[模块API](../../src/apps/web/src/spx/api/iam/management.ts)：实际Zod模型校验、CSRF/同源/取消与安全错误分类；内部authorize不进入浏览器 |
| Query/RHF | [角色页面](../../src/apps/web/src/spx/pages/roles.tsx)、[表单组合](../../src/apps/web/src/spx/components/forms.tsx)、[Table](../../src/apps/web/src/spx/components/data.tsx)：原生Query调用和身份/授权版本缓存键，RHF + 模块Zod，受控服务端分页；密码走模块 API和短期表单状态，不进入MutationCache |
| 国际化 | [英文](../../src/apps/web/messages/en.json)、[中文](../../src/apps/web/messages/zh-CN.json)：复用next-intl，不在业务组件判断语言 |
| 验证 | [业务集成](../../src/apps/iam/internal/service/authorization_integration_test.go)、[HTTP集成](../../src/apps/iam/internal/handler/server_integration_test.go)、[浏览器](../../src/apps/web/tests/e2e/identity.spec.ts)：真实PG/Redis、TLS、两个IAM副本、角色配置及影响预览 |

不要从按钮可见性推导服务端授权。列表allowed_actions是界面投影，实际写入仍读取最新数据库状态。Query只存可重取投影；退出/切组织先取消并清空，切换后的旧响应不能回写。503保持依赖不可用，不伪装成未登录。

新增业务沿这条链路扩展实际所属模块。Console拥有安全运营业务及必要的运营汇总，不复制IAM CRUD或成为所有模块的强制网关。成熟框架承担通用机制，项目保留权限、状态、并发版本、审计及响应策略。

本次设计参考gin-vue-admin的Gin分组、中间件和集中装配思路，未复制其业务、数据库可编辑API目录、自动改库或代码生成后台。研究版本及取舍见[框架能力记录](../plans/gin-service-refactor/framework-capabilities.md)。

管理变更后的缓存策略统一在[managementChanged](../../src/apps/web/src/spx/iam/management.ts)：Mutation 的成功回调先刷新身份，再通过 Query 失效当前客户端全部可重取投影（含非活动分页、角色目录和审计），避免只 refetch 当前表格。角色分配目录使用原生 Query 与身份/授权版本缓存键；角色信息表单中的本地副本只用于编辑。含密码的创建/重置操作直接调用 模块 API，成功后复用同一失效策略，凭据不进入 MutationCache。

九项审查修复后，角色创建使用 APIRoleCreate，更新使用 APIRoleInput（name/status/version 必需，不含 code）；预览使用原生 APIPreviewInput/APIPreview。集合结构由 Gin/validator 校验，表单 Zod 只负责交互校验，业务授权、状态、版本和事务仍归 IAM。身份范围变化会移除旧缓存，普通同范围写入继续由 managementChanged 统一失效关联投影。Axios 校验失败不会执行 Mutation 成功回调。
