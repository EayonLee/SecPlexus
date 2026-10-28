# 实施计划

状态：历史实施记录；后续整体重构由 [平台服务架构计划](../platform-service-architecture/plan.md) 承接。旧记录不表示新方案已实施。

- [x] 参考公开项目和 Go 规范，检查现有接口和调用方。
- [x] 删除项目内 .gitkeep 占位文件。
- [x] 为每个服务建立 router 入口，按业务分组 IAM 路由，并按职责拆分管理处理文件。
- [x] 收敛当前共用的错误码和 JSON 错误输出，保持既有成功响应及流式契约。
- 历史整理项已由 [平台服务架构](../platform-service-architecture/plan.md) 与 [代码规范技能](../spx-code-standard-skill/plan.md) 承接；不再以旧实现重复列待办。
- [x] 验证接口契约、单元、真实集成、浏览器核心流程及隔离 Compose。

参考：[Gitea 路由](https://github.com/go-gitea/gitea/blob/main/routers/web/web.go)、[Kratos HTTP 错误编码](https://github.com/go-kratos/kratos/blob/main/transport/http/codec.go)。

验证：单元测试、真实 PostgreSQL/Redis 集成（含 race）、6 项 Chromium E2E、契约检查、构建及隔离 Compose 通过。变量名改为 `SPX_POSTGRES_MODELESS`，独立配置测试验证该值进入驱动连接参数。

尚需继续：全项目代码与目录的逐项审阅、剩余长 SQL 和拥挤函数的可读性整理；如有跨域部署需求，需先确定可信来源及 Cookie/CSRF 策略，再实现带凭证跨域。

## 代码和依赖复核

- 按 Gitea 的路由分组和 Grafana 的响应边界核对本项目实现；当前路由按公开、内部及业务分组，错误响应由共享包处理。
- 将权限目录及可委派权限计算移入 IAM 应用层，补充管理员权限目录的真实集成验证；HTTP 层只负责读取请求和写入响应。
- 已按锁文件安装 Go 和前端依赖，修复前端格式检查；手写 Go 方法之间补齐空行，生成代码保持不动。
- 尚待继续：部分列表查询仍在 transport 层直接使用 SQL，且有较长的 SQL 和初始化表达式。需要逐个梳理并保持权限、分页和事务行为，不能声称整个项目的架构审查已完成。
