# 接口契约

本目录维护模块间公开协议，不能引用应用私有实现、ORM 或部署凭据。

- `common/openapi.json`：公共 Result 字段定义。
- `common/error-codes.json`：稳定 code、默认公开 message、HTTP status 和归属的唯一注册表。
- `<module>/openapi.source.json`：模块手写协议源，引用公共定义。
- `<module>/openapi.json`、`api.gen.go`：生成的独立协议文档和 Go 类型。
- `iam/permissions.json`：当前权限目录；对应 Go/TS 常量由生成器维护。

通过 `pnpm contracts:generate` 更新所有派生产物，不能手改 `.gen.*`。现有生成入口会合并公共 schema 后执行 oapi-codegen/openapi-typescript，Web 从同一协议生成类型与结果码。`pnpm contracts:check` 对比实际挂载路由和 OpenAPI，并检查权限与结果码注册表。

普通 JSON API 响应示例：

```json
{"code":"ok","message":"成功","data":{"id":"..."},"request_id":"..."}
```

失败仍返回真实 4xx/5xx，`data` 为 null，前端根据 code 决定恢复动作；message 只作公开提示，不包含内部错误。Go 使用 `result.Success/Created/NoContent/Fail`；自定义 code 先登记，必要公开消息通过 `FailWithMessage` 覆盖，不在 handler 散落 code/message/status 组合。

204、SSE、文件和第三方协议保持各自语义，不强塞 JSON。后续其他语言实现同一 wire contract，不共享 Go 类型或另造信封。公开路径为 `/api/v1/{module}/...`，内部路径为 `/internal/v1/{module}/...`，内部接口仍须认证与隔离。
