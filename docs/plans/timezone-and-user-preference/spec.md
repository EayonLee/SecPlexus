# 全平台时区统一与用户时区偏好

状态：已实施。范围仅时区语义与用户偏好；不引入按范围配置的时区，不做业务排班/报表的时区列。

## 背景与调研依据

多国用户协作时，同一份审计日志需要按各自本地时间阅读。经过对 Grafana、Sentry、AiSOC 的源码核对，业界共识与服务端配置相反：

- Sentry `src/sentry/conf/server.py` 明确 `TIME_ZONE = "UTC"`，注释为 "This should always be UTC"，并设 `USE_TZ = True`。
- Grafana `conf/defaults.ini` 的 `default_timezone = browser`，服务端不持有用户时区；`pkg/services/preference/timezone.go` 接受 `""`、`utc`、`browser` 或任意合法 IANA 名称，用 `time.LoadLocation` 校验。
- Grafana `pkg/services/preference/prefimpl/pref.go` 的覆盖顺序为：实例默认值 → 用户偏好逐级覆盖。
- AiSOC 的 `SettingsView.tsx` 有 timezone 字段，但只存 `localStorage`，全站没有用它格式化任何时间，属于未完成的装饰性字段，不作为参考实现。

结论：服务端固定 UTC，时区只是渲染层属性。禁止用 `TZ` 环境变量统一服务端时区，理由见「明确不做」。

## 实施前现状

- 数据库所有时间列已是 `timestamptz`，写入用 `now()` / `clock_timestamp()`，存储层无需改动。
- Go 侧共 8 处 `time.Now()`，均为耗时统计（`started := time.Now()`），无业务语义。
- `src/apps/web/src/i18n/config.ts` 硬编码 `export const timeZone = "UTC"`，`request.ts` 把它交给 next-intl 的 formatter，因此全站时间是 UTC，与用户所在地无关。
- `src/apps/web/src/spx/pages/management.tsx:150` 用该 formatter 渲染审计日志时间，是当前唯一受影响的用户可见时间。
- `iam.users` 表无时区字段。`/api/v1/iam/me` 由 `service/self.go` 的 `Identity` 返回 `map[string]any`，加字段不影响既有消费者。

## 范围

本计划实现八项：

1. 在 `AGENTS.md` 固化服务端 UTC 规则。
2. Web 显示层改用浏览器时区。
3. `iam.users` 增加 `timezone` 列。
4. `/api/v1/iam/me` 返回 `timezone`。
5. 设置页增加时区选择。
6. 时区校验复用 Grafana 的语义集合。
7. 偏好存数据库，不存 localStorage。
8. 显示回落顺序为「用户偏好 → 浏览器 → UTC」。

明确不做：

- 不加 `SPX_TIMEZONE` 环境变量，不改容器 `TZ`，不改镜像时区。理由：`TZ` 只改变进程本地时区默认值，会让 Go 的 `json.Marshal(time.Now())` 输出带本地偏移，与库中 UTC 不一致；日志时间戳与数据无法直接比对；同一份数据在不同部署序列化结果不同，可能影响缓存键、签名与幂等判断。且它无法表达「两个用户看不同时间」这一真实需求。
- 不做按范围配置的时区。用户已明确只需要用户级。
- 不在本轮为报表、排班、playbook 增加业务时区列，见「后续」。

## 行为契约

### 服务端时间规则

- 所有数据库时间列使用 `timestamptz`。
- Go 写入前显式 `.UTC()`；序列化到 HTTP 响应或日志时保持 UTC 或带偏移的 RFC 3339，不得输出无时区的本地时间。
- 服务端进程时区不参与业务计算，任何代码不得依赖 `time.Local`。
- 持续时间语义（会话空闲超时、锁定窗口、保留期）使用 interval 或 `time.Duration`，与时区无关。

### 用户时区偏好

- 字段：`iam.users.timezone text`，可空。空值表示未设置。
- 合法值：空字符串、`utc`、`browser`，或可通过 `time.LoadLocation` 加载的 IANA 名称（如 `Asia/Shanghai`）。其他值返回 400。
- 校验在服务端进行，不信任客户端；前端下拉框同样只提供合法选项。
- `browser` 是语义值而非时区名：表示「按访问者浏览器时区显示」，服务端不解释它。
- 存储位置：数据库，随账号迁移。禁止只存 `localStorage`（AiSOC 的缺陷：换设备即丢失）。
- `PATCH /api/v1/iam/me` 接受 `timezone`，返回更新后的值；写入成功需记录审计事件。

### 显示回落

Web 端解析顺序：用户 `timezone` 非空 → 使用它（`browser` 交给运行时解析为浏览器时区）→ 用户为空时使用浏览器时区 → 浏览器不支持或无法解析时回落 `UTC`。

切换时区后，所有使用 formatter 的时间立即按新时区重渲染，无需刷新页面、无需重新登录。

### 边界

- 多标签页：时区跟随用户偏好，不随页面或标签页切换。
- 未登录页面（登录页、错误页）：使用浏览器时区。
- 服务端渲染首屏与客户端水合后的时间必须一致，不得出现闪烁或水合不匹配。

## 验收

- Go：时区校验的合法/非法边界有表驱动单测，覆盖空串、`utc`、`browser`、合法 IANA、非法字符串、注入尝试。
- Go：集成测试验证 `PATCH /me` 写入后可读回、非法值返回 400、审计事件落库。
- Web：单元测试覆盖回落顺序的四个分支（用户值、`browser`、空+浏览器、都无法解析）。
- Web：审计日志在两种时区下渲染出不同且正确的本地时间。
- 浏览器实测：切换时区后时间立即变化，刷新后保持，换设备（清 localStorage）后仍保持。

## 后续（不在本计划）

出现定时类业务实体时，该实体自带 `timezone` 列，建的时候从创建者用户时区带入，执行时 `time.LoadLocation` 后换算为 UTC 存储。适用于「每天 9 点发日报」「每周一早班 8 点」「playbook 定时触发」。持续时间语义（如「30 分钟未处理升级」「保留 90 天」）与时区无关，不加该列。

依据：Sentry `src/sentry/monitors/models.py` 的 monitor 表独立持有 `timezone` 字段，因为「9 点」是业务语义，必须绑定创建者所在时区。
