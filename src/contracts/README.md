# 接口契约

协议不依赖应用私有实现、ORM或部署凭据。OpenAPI是唯一HTTP协议源，Gin是实际HTTP框架，二者承担不同职责。

| 文件 | 所有权 |
| --- | --- |
| `common/openapi.json` | 普通Result的四个wire字段 |
| `common/error-codes.json` | 稳定code、公开消息、真实HTTP状态与模块归属 |
| `<module>/schemas.source.json` | 模块共享schema；公开/内部协议按需引用 |
| `<module>/openapi.source.json` | `/api/v1/{module}/...`公开操作与权限元数据 |
| `iam/internal.source.json` | 独立TLS监听的服务间authorize协议 |
| `iam/permissions.json` | 权限目录与委派/近期认证约束 |
| `*.config.json` | oapi-codegen接入与共享类型映射 |
| `generated-files.json` | 生成器拥有的完整文件集合，包括尚未跟踪的新文件 |

`pnpm contracts:generate`在临时目录执行全部生成，全部成功后发布文件并清除清单内过时文件。`pnpm contracts:check`检查真实Gin路由/目录，再以`--check`重新生成比较文件集合和内容；校验模式不改工作区。不手改任何生成物，也不通过暂存或Git diff掩盖新文件。

生成链路：Redocly 2.7.0标准bundle/移除未使用schema → oapi-codegen 2.8.1共享Go类型、公开Gin接口和内部strict server；Web内的Hey API 0.99.0生成TS、Fetch SDK、Zod与Query options。版本由锁文件及生成配置固定。Web只输入公开bundle，不能得到内部路径、服务凭证或authorize请求类型。SSE保留原生Gin注册，不能套strict JSON响应包装。

运行时使用官方Gin/OpenAPI中间件。验证后的JSON值通过`runtime.BindJSONValue`绑定，避免第二次JSON解析。通过官方BodyDecoder扩展要求唯一JSON值；默认解码器允许尾随值，不能直接依赖默认行为。普通体积上限32KiB，登录/二次认证2KiB；分块输入同样受限。未知字段、错误媒体类型和非法参数在边界拒绝，具体权限和事务约束仍属于业务层。

日期是RFC3339，生成Zod配置允许时区偏移、拒绝无时区日期。JSON计数是非负安全整数；Go投影可使用int64，但不能把JSON计数转换成BigInt破坏Result的allOf组合。身份/授权版本继续使用十进制字符串，避免浏览器丢失精度。

普通结果：

```json
{"code":"ok","message":"成功","data":{"id":"..."},"request_id":"..."}
```

失败返回真实4xx/5xx，data为null；消息不包含内部错误。业务错误进入`c.Error/Abort`，由IAM统一映射。`src/packages/go/result`只构造业务信封和保证编码失败时尚未提交状态；内部strict响应使用生成visitor。204、SSE、文件和第三方协议保留自身语义。

新增接口先改source、登记权限/错误码，再生成并实现生成接口；不要另写影子路由表、TS-only DTO或相同字段的schema。实际开发范例见[角色管理](../../docs/development/role-management.md)。
