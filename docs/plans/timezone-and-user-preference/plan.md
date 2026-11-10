# 实施与验收

依据 [spec.md](spec.md)。按测试先行推进：每项先写失败测试，再实现。

## 依赖关系

第 1、2 项互不依赖，可先做，完成后多国用户即可看到各自本地时间，后端零改动。
第 3 至 8 项构成一条链：加列 → 接口 → 校验 → 前端选择 → 回落 → 不存本地。必须按序。

## 任务

### 第一组：立规则与改显示（可立即执行）

- [x] `AGENTS.md` 的 Testing 或 conventions 段落增加服务端时间规则：`timestamptz`、写入前 `.UTC()`、禁止 `time.Local`、禁止用 `TZ` 改服务端时区、持续时间用 interval。同时在该处指向本计划，作为时区语义的唯一说明。
- [x] 先写失败用例：断言 formatter 在浏览器时区非 UTC 时渲染出该时区时间。
- [x] 改 `src/apps/web/src/i18n/config.ts`，把硬编码 `timeZone = "UTC"` 改为由请求上下文解析的时区；`request.ts` 相应传入。
- [x] 验证 `management.tsx` 审计日志时间随浏览器时区变化。
- [x] 检查服务端渲染与客户端水合一致，无闪烁。

### 第二组：数据与接口

- [x] 先写失败集成测试：`PATCH /me` 带合法时区返回 200 且可读回；非法值返回 400；写入产生审计事件。
- [x] 新增迁移 `iam.users` 加 `timezone text`，可空，附列注释说明合法值与语义。
- [x] 实现时区校验函数，接受空串、`utc`、`browser`、合法 IANA；非法返回 `ErrInvalid`。
- [x] `service/self.go` 的 `Identity` 返回值增加 `timezone`。
- [x] 新增 `PATCH /api/v1/iam/me` 路由与 DTO，写入审计事件。

### 第三组：前端选择与回落

- [x] 先写失败单元测试：回落顺序四个分支。
- [x] 设置页增加时区选择，选项来自 `Intl.supportedValuesOf("timeZone")`，另加「跟随浏览器」与「UTC」两项语义值；不手写时区清单。
- [x] 提交后调 `PATCH /me`，成功后使相关查询失效并重渲染时间。
- [x] 确认不写入 `localStorage`。

## 验证入口

```sh
go test ./src/apps/iam/...
go test -tags=integration ./src/apps/iam/...          # 需 SPX_TEST_POSTGRES_URL 或 --docker
pnpm --dir src/apps/web test:unit
pnpm --dir src/apps/web exec tsc --noEmit
node scripts/build.mjs
```

浏览器实测项：切换时区后时间立即变化；刷新后保持；清 `localStorage` 后仍保持（证明存的是数据库）。

## 验收证据

实施后在此记录。未完成前不得标记完成。

- 单元测试：`tests/unit/timezone.test.ts` 11 个用例通过（回落顺序、语义值、非法值、偏好 cookie 序列化与清除）；`identity/timezone_test.go` 11 个表驱动子用例通过。
- 集成测试：`TestProfileTimeZonePreference` 通过（真实 PG/Redis）。覆盖写入四个合法值可读回、三个非法值返回 400 且不改变既有值、空串清除、会话 ID 不变。
- 类型与构建：`tsc --noEmit` 0；`vitest run` 17 文件 51 测试通过；`go build`/`go vet`/`go test` 全 0；根 `node scripts/build.mjs` 0；`pnpm catalogs:check` 0；`gofmt -l src` 无输出。
- 浏览器实测：已完成（chrome-devtools，真实登录 admin）。切换 UTC 后审计日志显示 UTC 时间，与本地时间相差 8 小时；刷新后保持；清掉偏好 cookie 后重开仍按已保存偏好渲染；切回「跟随浏览器」恢复本地时间。此过程发现并修复一个真实缺陷：清掉偏好 cookie 后首屏曾回退浏览器时区，现 `request.ts` 在镜像缺失时回源 `GET /api/v1/iam/me` 读取数据库真值。
- E2E（Playwright，隔离栈）：13 个用例全部通过，含新增「展示时区可切换并在刷新与清除本地缓存后保持」。时区用例按 `request_id` 锁定同一条审计记录，跨时区比较渲染结果，刷新与清 cookie 后仍一致。
- 顺带修正：审计表头原写死「时间（UTC）」，时区可配置后该文案错误，已改为「时间 / Time」。

实现说明：服务端读不到浏览器时区，因此使用两个 cookie，职责分离：

- `spx-timezone`：浏览器时区镜像，由 `timezone-boot.tsx` 在 hydration 前写入，只反映访问设备。
- `spx-timezone-preference`：用户已保存的偏好镜像，真源在数据库。由 `acceptIdentity` 与 `clear` 在身份投影变化的唯一安装点同步，避免只在设置页写入导致换设备读不到偏好、换账号继承上一人的时区。

`request.ts` 按「偏好 cookie → 浏览器 cookie → UTC」解析后交给 next-intl formatter，避免首屏 UTC 与本地时区不一致造成的闪烁。首次访问两个 cookie 都不存在时回落 UTC 一次。

同步写入位于浏览器环境，node 环境（单元测试）下跳过，与同文件既有的 `BroadcastChannel` 判断同一模式。

## 风险与约束

- 迁移为新增可空列，向后兼容，不需要回填。既有用户时区为空，显示回落浏览器时区，行为变化是「从 UTC 变成用户本地」，属于预期改进，需在发布说明中提示。
- 若前端在 `Intl.supportedValuesOf` 不可用的旧浏览器上运行，下拉框退化为「跟随浏览器 / UTC」两项，不报错。
- 不修改任何既有时间列类型，不动 `timestamptz`。
