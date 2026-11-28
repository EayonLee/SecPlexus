# 解析规则基础字段校验实施计划

状态：已实施。范围以[规格](spec.md)为准。

## 设计依据

| 来源 | 对 SecPlexus 的用法 |
| --- | --- |
| Elastic Fleet | 集成策略区分名称、ID 和可选描述。简化请求模型分别限制名称和描述长度，保存时清理名称空白并检查重名。复用约束分层思路，数值沿用本平台已有边界。[请求模型](https://github.com/elastic/kibana/blob/main/x-pack/platform/plugins/shared/fleet/common/types/models/package_policy_schema.ts) · [保存流程](https://github.com/elastic/kibana/blob/main/x-pack/platform/plugins/shared/fleet/server/services/package_policy.ts) |
| Graylog | 规则 API 可单独试解析，保存前仍会解析规则；描述独立存储。样本提取和管理写入继续各自负责，保存时不依赖客户端的试算结论。[规则 API 源码](https://github.com/Graylog2/graylog2-server/blob/master/graylog2-server/src/main/java/org/graylog/plugins/pipelineprocessor/rest/RuleResource.java) |
| PostgreSQL | 唯一索引处理并发写入下的最终查重。沿用 `ingest.mappings.name_normalized` 与唯一索引。[唯一索引](https://www.postgresql.org/docs/current/indexes-unique.html) |
| 本地 Dashboard | Products 的 Add Product Drawer 将表单 schema、字段状态、`FieldError` 和错误关联 ID 放在一个表单流程内。SecPlexus 保留向导布局，复用这套提示方式及现有共享表单组件。[Web 模板映射](../../../src/apps/web/README.md#selective-template-adoption) |

## 校验责任和错误契约

1. **Ingest 拥有业务约束。** 在模块内为规则名称、来源文本和描述集中定义限制与归一化。Gin 解码及校验 JSON 类型，创建和更新接入同一个规则元数据校验入口。厂商和产品的样本验证入口沿用相同的来源文本规则。写入时在规则定义的解码结果上校验描述，随后编译同一份定义。运行时编译仍接受历史修订中超限但结构合法的描述，避免外部配置刷新失败。数据库写入使用归一化后的值，保留现有不可变规则修订。字段契约目录继续负责 Event 字段约束。
2. **Web 使用 RHF 和 Zod。** Ingest 表单共享来源字段 schema，规则向导复用基础字段 schema。前后端按 Unicode White_Space 规则去除首尾空白，按码点计长，描述的 CRLF 与 CR 统一为 LF。使用同一表驱动边界用例核对 Go 和 Web 的接受结果。现有 HTML `maxLength` 的 UTF-16 计数由 schema 校验和字段提示接替，避免粘贴被静默截断。共享表单控件负责错误呈现，表单所在业务模块负责哪一个字段出现哪条错误。
3. **HTTP 延用公共结果封装。** 为已有错误响应增加可选的 `field_errors` 列表，每项只含 `field`、`code` 与必要的有限参数，例如长度上限。继续使用 `invalid_request`（400）和 `conflict`（409），保留 `request_id` 与 `data: null`。Ingest 将名称唯一索引冲突归为 `name / duplicate_name`，将版本冲突归为 `expected_version / revision_conflict`。Axios 对错误明细做 Zod 解码，Web 用白名单映射到 RHF 和 next-intl。响应不回显原始坏值、SQL 或驱动错误；未知明细回落到原有提示。
4. **数据库兜底。** 沿用名称长度约束、归一化列及唯一索引。拟通过前向迁移补充厂商、产品长度及 JSON 内描述的类型和长度检查，更新相关列注释。先用只读检查列出已超限规则 ID，业务确认后走管理写入修订并记录审计。上线先更新服务写入校验，再以短事务登记 `NOT VALID` 约束并单独验证，防止扫描期间长期阻塞写入。历史修订表保持不可变。[迁移规范](../../development/database-design.md) · [PostgreSQL ALTER TABLE](https://www.postgresql.org/docs/current/sql-altertable.html)

重名判定走现有索引，写事务沿用原来的修订与审计提交边界。两个并行请求使用同名时一项成功，一项返回重名。字段编辑期间做本地校验，服务端样本验证复用来源文本校验，查重在保存时由数据库确认。字数与控制字符检查按字段线性扫描，已有请求体上限继续生效。正常入流沿用当前已编译的规则。

### 升级前检查

在应用 PostgreSQL 迁移 00010 和 00011 前执行以下只读查询。若有结果，先确认并通过管理写入修订对应规则。

```sql
SELECT id
FROM ingest.mappings
WHERE char_length(vendor) > 120
   OR char_length(product) > 120
   OR (definition ? 'description' AND (
       jsonb_typeof(definition->'description') IS DISTINCT FROM 'string'
       OR char_length(definition->>'description') > 1000
   ));
```

## 实施与验收

1. 写跨前后端边界用例，覆盖空值、全空白、中文和补充字符、长度正好到边界及超出一位、控制字符、多行描述。测全新规则、复制内置规则和修改既有规则。
2. 在真实 PostgreSQL 验证重复提交、不同大小写或首尾空白的重名、保留原名称，以及并发同名只产生一次成功修订。分别验证旧版本冲突、重名冲突及非法字段的状态和明细。对新增迁移验证空库、既有记录、失败及回滚。
3. 用 Chrome 验证字段错误的位置、键盘焦点、前后端反馈一致、重名时草稿保留和重新保存，覆盖桌面与窄窗口。运行 Ingest、Web、迁移和契约测试，更新 Ingest README 的现行限制。

已通过 Ingest 和 Web 测试、真实 PostgreSQL 并发与迁移回滚检查、sqlc 和迁移边界检查。Chrome 验证了桌面和窄窗口的提示与焦点，隔离端到端验证覆盖重名修正、修订冲突、规则复制、规则编辑和设备推送。
