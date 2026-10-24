# 04 · 调研证据库

本文档汇集架构决策背后的全部证据：国内云厂商对照、海外 AI SOC 公司对照、开源参照项目、学术评测数据、失败教训清单。证据强度标记沿用 README 的约定（【官方文档】【代码】【论文】【实测】【推断】【未核实】）。

## 1. 国内厂商：告警运营链路对照

| 厂商 | 运营对象 | 归并方式 | AI 研判 | 出处 |
|---|---|---|---|---|
| 阿里云（安全事件处置 / Agentic SOC） | 安全事件 | 图计算聚合关联告警，还原攻击链提取恶意实体 | 安全运营 Agent（自主感知-推理-执行） | 【官方文档】help.aliyun.com |
| 腾讯云 SOC（调查中心） | 安全事件 | 按影响资产、时间线、ATT&CK 战技术关联，自动还原攻击过程 | AI Copilot 辅助 | 【官方文档】cloud.tencent.com/document/product/1011 |
| 腾讯云 CSC | — | — | 四核心理念：统一资产基座（含 **AI 资产**）/安全左移/运行时防护/运营闭环 | 【官方文档】cloud.tencent.com/document/product/664 |
| 火山引擎安全运营智能体 | **实体事件** | 规则前置过滤 → 相似归并 → **AI 以组为单位研判** → 攻击链串成事件 | 三队列（AI已研判等你授权 / AI不确定需要你的认知 / 已闭环不打扰你）+ 记忆 + 知识前置过滤 + 深度调查 | 【官方文档】docs.volcengine.com |
| 奇安信 NGSOC（经 flocks 对接） | 合并告警 | **四种 ID 并存**：alarm-id / raw-alarm-id / ticket-id / merge-alarm-id；处置接口 `alarm_dispose_state` 要的是 `alarmIds`（原始告警 uuid），AI 研判用 merge-alarm-id | AI 研判（SSE） | 【代码】flocks `.flocks/plugins/tools/device/ngsoc_v4_15_1/ngsoc_alarms.yaml` |

**关键结论**：平台型全部有事件层（工具型如 flocks 才是纯告警）；连国产 SIEM 自己都区分四种 ID——处置回写必须用上游原生 ID 是硬结论。

### 腾讯云鼎的真实数字【官方文档】

- 7 天 17 万原始告警 → 同特征聚合 1834 条 → Agent 研判后 42 条（总降噪 97.7%）
- **单 Agent 成功率仅 75%**，官方自述原因：上下文爆炸、60+ 工具选错、"1/4 概率连结果都输出不了"
- 真实案例：一条"攻击失败的 RCE"告警，Agent 查日志发现 php-fpm 两个月前执行过反弹 Shell 且进程仍在——**结论靠翻日志，不靠告警本身**（"只有告警"能力边界的实证）

## 2. 海外 AI SOC 公司对照

| 公司 | 编排路线 | 关键设计 | 定价 |
|---|---|---|---|
| [Dropzone AI](https://www.dropzone.ai) | 自由递归推理 | 假设驱动调查；Context Memory 三来源（调查/纠正**强制填原因**/直录）；Schema 适应查询；数据血缘（每个问题/工具/结果可导出，链回原始日志）；"组装 vs 判断"；AI 只调查不响应 | $36K/年 = 4000 次调查 ≈ $9/次，无限用户 |
| [Prophet Security](https://www.prophetsecurity.ai) | **图编排 + 受限 LLM** | "一致且确定，而非自由形式 LLM 链"；每问题/查询/推理步骤可审计；单租户部署；AI Detection Engineer 从用户数据建 ATT&CK 覆盖图 | 反对 per-seat（"成本跟人头不跟工作量"） |
| [Exaforce](https://www.exaforce.com) | 4 个对等 Exabot | 统一数据层 + **实时知识图谱**做底座；95% 告警 AI 处理 | — |
| [Torq](https://torq.io/socrates) | omni-agent + HyperAgents | Socrates 编排 + Context Graph（环境记忆） | ~$60K |
| Radiant（2026-08 被 Cribl 收购） | 模仿人类分析师 | FP 降 98%；"每分析师每天只升 1–3 条高保真告警" | — |
| [Intezer](https://intezer.com) | 证据流水线 | 收集→分析（递归解包）→关联（含**组织可信制品库**）→证据级 verdict→聚合告警级 verdict | — |
| Microsoft Security Copilot | 单职责 agent 套件 | phishing/alert triage、TI、KQL hunting 等各管一摊；每 agent 自治独立配置；责任 AI 护栏；用户实测月省约 200 小时 | — |
| Google SecOps (2026) | 专业 agent | Detection Engineering agent + Triage agent | — |

**Radiant 被 Cribl（数据管道公司）收购**的信号：数据层是根基、AI 是放大器——独立 AI SOC 在收敛，要么长在自己的数据面上，要么被数据面公司吃掉。这支撑我们自建数据面（collector/归一化/实体档案）的路线。

### 失败与负面教训

| 教训 | 出处 | 对我们的含义 |
|---|---|---|
| MS Phishing Triage Agent 被一线用户评 **"mid"**：明显钓鱼 OK，**低慢攻击差**；被批"改进邮件网关就不需要这个 AI" | 【实测】Reddit r/DefenderATP | 低慢攻击是系统性盲区，显式写进能力边界；AI 不做上游数据质量烂的补丁 |
| Langfuse v3 拆 web/worker 后用户忘起 worker，摄取**静默失败** | 【实测】GitHub discussion #11216 | standalone=all 免疫；compose/helm 必带 healthcheck + 依赖条件 |
| AiSOC 归一化器把上游 `count` 整个吃掉 | 【代码】`normalizer.go` 搜 "count" 零命中 | 聚合语义字段必须读进内部模型 |
| AiSOC 时间解析失败回退 `time.Now()`，无告警 | 【代码】`normalizer.go:1063` | 时间失败 → null + warning，绝不覆盖 |
| AiSOC groundedness scorer 曾"只在评测里跑、生产无调用方"，自信误报被自动关单 | 【代码】`test_groundedness_gate.py` 文件头自述 | 有指标 ≠ 有门禁，评测分数必须接进生产链路 |
| 配置了关单策略的租户反而被锁死（查询写错表名） | 【代码】ADR-0009 | "谨慎的配置比不配置更危险"是真实发生过的缺陷形态 |

## 3. 存储与冷热分层业界全景

| 厂商 | 冷层叫什么 | 冷层能否检索 | 冷层能否 SQL 分析 | 延迟 | 并发 |
|---|---|---|---|---|---|
| 阿里云 SLS | 低频/归档存储 | ✅ | ✅ **能** | 低频百 ms–秒；归档**分钟** | 归档层**查询 1 / 分析 1** |
| 腾讯云 CLS | 低频存储 | ✅ 仅全文 | ❌ **不能**（无键值/仪表盘/告警） | 亿条全文 2s | — |
| 火山引擎 TLS | 归档存储 | 未核实 | 未核实 | 第三方称分钟级（不采信） | — |
| Splunk SmartStore | warm（对象存储） | ✅ | ✅ | 缓存命中≈热层；**97% 搜索回看 ≤24h**（缓存策略依据） | — |
| Elastic | cold / frozen | ✅ | ✅ | frozen p99.9：未缓存 2.3–13s / 已缓存 1–11s；最差首查 5 分钟 | — |
| MS Sentinel | Data Lake tier | ⚠️ 只能 KQL job | ❌ **不能实时分析和威胁狩猎** | 摄入延迟 15 分钟 | — |
| Google SecOps | **不分冷热** | ✅ 12 个月全热态 | ✅ | 与查最近一小时相同 | — |

三条实现路径：A 统一热存储（仅 Google，不可复制）；B 冷层可查但降级 + **本地缓存层必需**（Elastic benchmark 证明无缓存的冷查询是灾难）；C 冷层=归档（腾讯/Sentinel）。按"工作记忆"存储模型（02 §6.1），告警只留 90 天工作窗口（≈9–27GB），冷热分层当前整体不需要；若未来规模超限（单租户 >100 万条/天或多租户 SaaS），沿路径 B 演进，本表备查。

## 4. 归一化工程缺陷清单（全部代码实证）

来自 AiSOC / flocks 源码逐行核实的真实缺陷，v1 全部写成 CI 门禁：

| # | 缺陷 | 出处 | 铁律 |
|---|---|---|---|
| 1 | 时间解析失败静默回退当前时间 | AiSOC `normalizer.go:1063` | null + warning，不参与时间线关联 |
| 2 | 字段路径不支持数组下标，映射静默失效 | AiSOC `getNestedField` 遇数组返回 nil，而 profile 写了 `sourceIPs.0` | 支持下标 + 填充率报表（0% = bug） |
| 3 | 缺失值用字符串 `"none"` | flocks `sangfor.py` `_pick()` | 缺失就是 null |
| 4 | 只按 UTF-8 解码，国内设备 GBK 全变乱码 | flocks `parser.py` `errors="replace"` | UTF-8 strict → GB18030 → replace，编码记在事件上 |
| 5 | `actor.kind` 瞎猜导致关联全错 | AiSOC 注释：11/68 连接器实体段塌成 "unknown"，**该连接器所有告警关联进同一个桶** | kind 判断不出就是 unknown |
| 6 | 指纹未排序导致去重失效 | AiSOC `fingerprint()` 修正后的做法 | list 字段进指纹必须排序；tenant_id 进指纹 |

## 5. 学术评测数据汇总

| 评测 | 设置 | 结果 | 含义 |
|---|---|---|---|
| ALERT-BENCH / AIDA（arXiv 2610.10608） | 1,247 告警、4 天多阶段攻击、Wazuh+MCP、GT 隐藏 | 五种主流范式漏报均 >40%；**AIDA 四角色 F1 0.958**，代价是升级率 8.5%→18.4%；去 Judge 掉到 0.895；9B 模型也 0.952 | **结构比模型重要** |
| 空结果陷阱（同上） | 检索无结果时 | 4/5 方法丢掉 ≥88% 攻击告警 | 工具三态，空 ≠ 干净 |
| Self-Refine / CoVe（同上引） | 同上下文自审 | Self-Refine 改对 16 / 改错 79；CoVe 25/132 | 禁止同上下文自审 |
| ExCyTIn-Bench（arXiv 2507.14201，Microsoft） | 57 张 Sentinel 表、7,542 题 | 最好模型 reward 0.606 | 调查能力远未饱和 |
| SIABench（arXiv 2603.06422） | triage vs 深度调查 | triage（TP/FP）96–98%；**深度调查 40–60%** | TP/FP 可自动化，溯源人兜底 |
| CTIBench（arXiv 2406.07599，NeurIPS'24） | 威胁情报 | ATT&CK 开放抽取 macro-F1 **0.3108**；组织归因 correct 52/plausible 86 | ATT&CK 只做候选集分类 |
| Let Me Speak Freely（arXiv 2408.02442，EMNLP'24） | 格式约束 | JSON 模式下 GPT-3.5 **100%** 把 answer 排在 reason 前 | evidence 排在 verdict 前 |
| CyberSOCEval（CrowdStrike×Meta） | 安全分析 | 推理模型拿不到 coding/math 那种 test-time scaling 红利 | 换贵模型不是解法 |
| 温度=0 仍不稳定（Muennohoff 引） | 确定性 | greedy sampling 输出仍显著变化 | 结果留档 + 可复现设计 |

### 5.1 行业研究：告警疲劳与处理能力缺口（定位的定量依据）

| 数据 | 出处 |
|---|---|
| SOC 平均日告警 4,484（2023）→ 3,832（2025）→ 2,992（2026）；**被忽略比例 67% → 63% 几乎未降**——量在降、处理能力没跟上 | 【行业研究】Vectra AI State of Threat Detection 系列 / [alert-fatigue](https://www.vectra.ai/topics/alert-fatigue) |
| 单条告警人工调查平均约 70 分钟 | 【行业研究】D3 Security |
| 四人 SOC 每周仅能深查 300–400 条高保真告警，90% 判为良性 | 【行业研究】Prophet Security |
| 某 Fortune 500 金融 SOC 日告警 15,000+，85% 误报 | 【行业研究】Torq 引述 |

这组数字是产品第一价值主张（提效减负）的定量表述：传统 SOC 交付了检测能力，没有交付处理能力。

### 5.2 竞争定位证据：第三方大脑的存在理由

| 论点 | 出处 |
|---|---|
| 厂商 AI 被自家遥测边界限制（"XDR Ceiling"），中立层坐在任何检测层之上 | 【官方文档】[D3 Security: The XDR Ceiling](https://d3security.com/resources/xdr-ceiling-autonomous-soc) |
| Dropzone 自我定位即 "Vendor-Agnostic AI Layer"，连接既有 SIEM/EDR/云/身份/工单，"a handoff, not a rip-and-replace" | 【官方文档】[dropzone.ai/blog/vendor-agnostic-ai-soc-layer](https://www.dropzone.ai/blog/vendor-agnostic-ai-soc-layer) |
| AI SOC 层与 SIEM 互补而非替代（SIEM 管摄取与检测，AI 层管调查） | 【官方文档】Dropzone "SIEM Alternatives" |
| 已有 123 家 AI SOC 厂商，大量只是往现有工具上糊 copilot | 【行业研究】[KuppingerCole](https://www.kuppingercole.com/blog/gardiner/123-ai-soc-vendors) |

### 5.3 AI 助手 vs AI-native 的证据

| 论点 | 出处 |
|---|---|
| AI-native = "AI 设计进架构、数据流、决策与治理的地面起方案" | 【行业研究】[HivePro: AI Native SOC](https://www.hivepro.com/blog/ai-native-soc-architecture-workflows-governance) |
| 在遗留 SIEM 上层叠 AI 不修复根本缺口，多数"AI SOC"因此失败 | 【行业研究】[databahn: What an AI-Native SOC Actually Looks Like](https://www.databahn.ai/blog/what-an-ai-native-soc-actually-looks-like) |
| AI-native（AI 为中心组件）与 AI-enabled（AI 为外围功能）的架构差异 | 【官方文档】[Exabeam: AI Native vs AI Enabled](https://www.exabeam.com/explainers/ai-cyber-security/ai-native-vs-ai-enabled-5-differences-and-security-considerations) |
| 国内厂商 AI 助手收敛为"通用 agent + CLI + prompt 方法论"结构 | 【推断】基于国内厂商产品公开演示与文档的多方观察归纳 |

## 6. 开源参照项目细节

### AiSOC（MIT，v20.1.0，2945★ 调研时）

- **存储 ADR-0005**：CH 热 30 天 / 对象温 60 天 / 冷 275 天；明确拒绝第二个热引擎；1TB/天 8×ZSTD 约 $902/月（自标"参考价必须重核"）
- **扩容 ADR-0012**：规则按必需字段索引（实测 2.6× 提速、68.3% 候选减少）；分区键 `tenant:entity`；**发布 lag 指标但不自动扩容**（"autoscaler 是集群的，不是应用的"）
- **门禁**：groundedness floor 0.75、只拦自动关单（不对称设计："降级升级只增加 review 负担"）
- **审批**：playbook 到审批点持久化挂起（migration 081），过期有记录结果
- **部署**：compose CORE(8GB)/full profiles + helm(HPA/PDB/values-ha) + install.sh 安装即验收

### flocks（MIT，Apache-2.0 系）

- SOC workspace = webui + 查询工具 + denoise/triage 两个 workflow 的组合包
- **8 类 workflow 节点**：python/logic/branch/loop/tool/llm/http_request/subworkflow（`models.py` 实证）
- `stream_alert_denoise`：4 节点纯 workflow（无 LLM），MinHash LSH 语义去重——**LSH 状态 649MB pickle 曾致内存线性增长**（子进程隔离的由来）
- `stream_alert_triage`：4 路并行 LLM 分支 + **dedup leader/follower**（同 key 只调一次 LLM）
- trigger 模型：manual/schedule/webhook/syslog/kafka/…（数据驱动的一切）

## 7. AI 网关与检测组件选型

| 组件 | 许可证 | 状态 | 结论 |
|---|---|---|---|
| [agentgateway](https://github.com/agentgateway/agentgateway) | **Apache 2.0 全开源**（LF 项目，微软/阿里/AWS/思科/华为参与） | 活跃，Rust 数据面 | ✅ ai-gateway 数据面首选；只依赖 xDS + ext_authz |
| [Higress](https://github.com/higress-group/higress)（阿里） | Apache 2.0，CNCF Sandbox | 活跃 | 国内云用户的备选 |
| [LiteLLM](https://github.com/BerriAI/litellm) | MIT 核心；**SSO/SCIM/密钥轮换在企业许可** | 活跃 | 仅作 SDK 内嵌于 agent-runtime（我们的 iam 在前，不需要它的 SSO） |
| [NeMo Guardrails](https://github.com/NVIDIA/NeMo-Guardrails)（NVIDIA） | Apache 2.0 | 活跃 | ✅ ai-policy 内嵌检测库 |
| LLM Guard（Protect AI） | MIT | ⚠️ **已归档停止维护** | 不选 |

## 8. OCSF 事实核对（schema.ocsf.io API 实查，v1.9.0）

- `security_finding`(2001)：`@deprecated since 1.1.0`，superseded_by 含 `detection_finding`
- `detection_finding`(2004)：77 字段，含 `count`/`start_time`/`end_time`/`duration`/`unmapped`/`vendor_attributes`/`evidences[]`/`observables[]`
- `incident_finding`(2005)：73 字段，`status_id` 必填、`verdict`/`confidence_id`（"summary across all aggregated findings"）/`assignee`/`ticket` —— 案件模型直接映射
- 生态：2024-11 入 Linux Foundation（200+ 组织、900+ 贡献者，2026-03 达 1,280）；2025-12 获 ITU 成员国支持、2026-06 成为国际标准；Atlassian 44 亿事件/天案例（成本降 80%、查询 45s→2-3s）
- v1.9.0 含 `ai_agent`/`ai_model`/`ai_operation` profile —— 大模型护栏支柱可复用标准 schema

## 9. 明确未核实清单（不编造）

1. 火山引擎 TLS 存储分层官方正文（文档站 SPA 抓取不到）
2. 腾讯云 CLS CQL 语法逐字原文（页面反爬，来自搜索摘要）
3. 华为云 SecMaster 未纳入
4. 各厂商"合并"的内部实现均未公开
5. Microsoft SCU 单价、各家单次研判真实 token 成本
6. CrowdStrike "validation agents" 说法未在官方页找到，不引用
7. SIABench 的 triage 96–98% 基于公开数据集，不可用于对用户承诺
8. 火山引擎"记忆/知识"模块的内部实现细节

## 10. 主要参考链接

### 调研期间分析过的本地仓库

- AiSOC（开源 AI SOC，18 服务）：github.com/beenuar/AiSOC
- flocks（开源 AI 原生 SecOps 平台）：本地克隆分析

### 标准与官方文档

- OCSF：`https://schema.ocsf.io` / `https://ocsf.io`
- 火山引擎安全运营智能体：`https://docs.volcengine.com/docs/Securityoperationagent`
- 腾讯云 SOC：`https://cloud.tencent.com/document/product/1011`
- 阿里云云安全中心：`https://help.aliyun.com/zh/security-center`
- GitLab Omnibus 数据库配置：`https://docs.gitlab.com/omnibus/settings/database`
- Elastic frozen tier benchmark：`https://www.elastic.co/search-labs/blog/searchable-snapshots-benchmark`

### AI SOC 公司与行业研究

- Dropzone 技术博客（假设驱动调查 / Context Memory / vendor-agnostic 定位）：`https://www.dropzone.ai/blog`
- D3 Security "The XDR Ceiling"：`https://d3security.com/resources/xdr-ceiling-autonomous-soc`
- KuppingerCole "123 AI SOC Vendors"：`https://www.kuppingercole.com/blog/gardiner/123-ai-soc-vendors`
- Vectra AI 告警疲劳研究：`https://www.vectra.ai/topics/alert-fatigue`
- HivePro "AI Native SOC: Architecture, Workflows, and Governance"：`https://www.hivepro.com/blog/ai-native-soc-architecture-workflows-governance`
- databahn "What an AI-Native SOC Actually Looks Like (and Why Most Fail)"：`https://www.databahn.ai/blog/what-an-ai-native-soc-actually-looks-like`
- Exabeam "AI Native vs AI Enabled"：`https://www.exabeam.com/explainers/ai-cyber-security/ai-native-vs-ai-enabled-5-differences-and-security-considerations`

### 开源组件

- agentgateway：`https://agentgateway.dev`（Apache 2.0，Linux Foundation）
- Higress：`https://github.com/higress-group/higress`
- LiteLLM：`https://github.com/BerriAI/litellm`
- NeMo Guardrails：`https://github.com/NVIDIA/NeMo-Guardrails`
