# 角色管理：实际开发贯通范例

本页链接当前实现，不复制一套会过时的演示源码。核心规则是组织隔离、实时授权、委派上限、版本冲突、系统角色保护和变更/审计同事务。

| 环节 | 当前文件与职责 |
| --- | --- |
| 协议 | [操作源](../../src/contracts/iam/openapi.source.json)、[共享schema](../../src/contracts/iam/schemas.source.json)、[权限目录](../../src/contracts/iam/permissions.json)：定义方法、路径、body、结果及action；修改后统一生成 |
| Gin接入 | [路由](../../src/apps/iam/internal/router/routes.go)、[管理Handler](../../src/apps/iam/internal/handler/management/roles.go)：实现生成接口，复用已验证值；错误加入Gin错误链，成功输出Result/204 |
| 安全与身份 | [入口装配](../../src/apps/iam/internal/handler/server.go)：公开会话/CSRF/限流/协议与内部服务认证隔离；HTTP context只携带请求状态 |
| 应用用例 | [角色用例](../../src/apps/iam/internal/service/roles.go)、[管理事务](../../src/apps/iam/internal/service/service.go)：Transaction回调内重验身份、组织/用户锁、权限和版本，变更及审计一起提交 |
| 仓储 | [角色关联](../../src/apps/iam/internal/repository/roles.go)：读取旧授权一次、批量插入新增关系；保留未变更的弃用授权，避免重新触发新增约束；成员关系批量写入仍受组织复合FK约束 |
| 数据类型 | [普通列表](../../src/apps/iam/internal/repository/lists.go)、[查询](../../src/apps/iam/internal/repository/queries.go)：GORM泛型和具名公开字段投影；特殊锁/数组查询保留必要参数化SQL，不建设BaseRepository |
| 客户端 | [Web生成配置](../../src/apps/web/openapi-ts.config.ts)、[SDK边界](../../src/apps/web/src/spx/iam/session.ts)：生成成功响应验证、CSRF/同源/取消与安全错误分类；内部authorize不进入浏览器 |
| Query/RHF | [角色页面](../../src/apps/web/src/spx/pages/roles.tsx)、[表单组合](../../src/apps/web/src/spx/components/forms.tsx)、[Table](../../src/apps/web/src/spx/components/data.tsx)：生成Query options和身份/授权版本tags，RHF+生成Zod，受控服务端分页；密码走SDK和短期表单状态，不进入MutationCache |
| 国际化 | [英文](../../src/apps/web/messages/en.json)、[中文](../../src/apps/web/messages/zh-CN.json)：复用next-intl，不在业务组件判断语言 |
| 验证 | [业务集成](../../src/apps/iam/internal/service/authorization_integration_test.go)、[HTTP集成](../../src/apps/iam/internal/handler/server_integration_test.go)、[浏览器](../../src/apps/web/tests/e2e/identity.spec.ts)：真实PG/Redis、TLS、两个IAM副本、角色配置及影响预览 |

不要从按钮可见性推导服务端授权。列表allowed_actions是界面投影，实际写入仍读取最新数据库状态。Query只存可重取投影；退出/切组织先取消并清空，切换后的旧响应不能回写。503保持依赖不可用，不伪装成未登录。

新增业务沿这条链路扩展实际所属模块。Console拥有安全运营业务及必要的运营汇总，不复制IAM CRUD或成为所有模块的强制网关。生成器和框架承担通用机制，项目保留权限、状态、并发版本、审计及响应策略。

本次设计参考gin-vue-admin的Gin分组、中间件和集中装配思路，未复制其业务、数据库可编辑API目录、自动改库或代码生成后台。研究版本及取舍见[框架能力记录](../plans/gin-service-refactor/framework-capabilities.md)。
