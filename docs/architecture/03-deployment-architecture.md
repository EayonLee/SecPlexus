# 03 · 部署架构

> 用大白话说：一个仓库、两个镜像（双架构）。基础组件默认捆绑开箱即用，用户想用自己的 PG/S3/Redis，改 `.env` 一行就切过去。从单机 docker 到 k8s 横向扩展不改任何代码，全生命周期一个 `spxctl` 命令管，信创内网给离线包。

这套部署架构的每个机制都有 ≥1w star 的大型开源项目验证过（Dify 159k、Supabase 114k、RAGFlow 93k、JumpServer 32k、GitLab、Langfuse 36k、Wazuh 17k），对照表见文末。

## 1. 仓库：monorepo 单仓

全部服务一个仓库开源，不分仓。参照：Supabase（114k★，apps/packages 同仓）、Sentry self-hosted（30+ 服务一仓管理）、AiSOC（18 个服务一仓）。

```
secplexus/
├── src/
│   ├── apps/                # 可独立运行的应用；当前仅创建三个模块的基础目录
│   │   ├── web/
│   │   ├── iam/
│   │   └── console/
│   ├── packages/            # 实际跨应用复用的库，按需提取
│   └── contracts/           # 接口与事件契约，不包含业务和数据库访问实现
│       ├── openapi/
│       └── proto/
├── deploy/
│   ├── local/               # 本地开发配置
│   ├── compose/             # Docker Compose 部署
│   └── helm/                # Kubernetes 部署
├── scripts/                 # 开发、构建与发布工具
├── docs/                    # 架构、模块设计与开发约定
├── go.mod                   # Go 应用共用的根模块
└── Makefile                 # 当前提供 help / check，运行与构建目标随实现添加
```

应用内部保留各语言自己的目录和依赖惯例。后续应用加入 `src/apps/`，
不提前创建空模块或共享平台库。当前实际目录与开发约定见
[开发文档](../development/README.md)。下文的 Python 多角色镜像方案为此前设计，
尚未与后续多语言、应用独立部署的讨论同步，不作为当前实现约束。

## 2. 镜像：单镜像多角色（SPX_ROLES）

CI 只产出两个镜像，都是 `linux/amd64 + linux/arm64` 双架构（Dify、RAGFlow 均如此，业界无一家默认做 loong64）：

- **`secplexus`**：全部 Python 角色，由 `SPX_ROLES` 环境变量决定起什么
- **`secplexus-web`**：Next.js（standalone 模式下由 console 托管构建产物，可不用）

这个模式业界叫 **role-based deployment**：Langfuse（36k★）用一个镜像跑 web/worker 两个容器；Docker 官方与 microservices.io 都认可它优于真正的 all-in-one。价值在于：**从单机到横向扩展只改编排，不改代码**。

自研镜像仅 **2 个**：`secplexus`（Python，全部 8 个角色；agent 因 LLM 故障隔离部署为独立容器，同镜像 `SPX_ROLES=agent`）、`secplexus-web`（TS）。standalone 模式 = secplexus 单容器跑 all（asyncio task group）+ console 托管 web 产物。

```
SPX_ROLES=collector          → 单角色容器
SPX_ROLES=correlator,agent   → 组合角色
SPX_ROLES=all                → 所有角色跑在一个 asyncio 进程（task group）
```

standalone 不用 supervisord：我们的角色全是 IO 型，一个 asyncio 进程用 task group 承载多个角色比多进程 supervisor 干净（Elastic 的 Python async 模式背书）。trade-off 明示：单进程里 CPU 密集步骤理论上互相挤占——standalone 定位小规模/POC/边缘，生产拆容器。

## 3. 部署架构总图

```
══════════════ ① 镜像层（CI 产出，双架构 amd64+arm64）═══════════════════

  ┌────────────────────────────────────┐    ┌───────────────────┐
  │  镜像: secplexus（SPX_ROLES 定角色） │    │ 镜像: secplexus-web│
  └────────────────────────────────────┘    └───────────────────┘
            │              │               │              │
            ▼              ▼               ▼              ▼

═══ 形态 A：docker 单容器 ══════════════════════ 形态 D：源码部署 ══════════

  docker run -e SPX_ROLES=all secplexus        spxctl install --source
  单进程 asyncio 全角色 + console 托管前端      （uv + 前台角色进程）
        │                                      定位：开发/无 docker 环境
        ▼ 外部基础组件（用户 env 指定）

═══ 形态 B：docker-compose 全家桶（默认形态）══════════════════════════════

  ┌─ spxctl install ── 读 docker/.env ── 起栈 ── 安装即验收（golden path）──┐
  │                                                                     │
  │   ┌──────────── 应用容器（同一镜像，SPX_ROLES 区分）────────────┐        │
  │   │  spx-api    spx-collector   spx-correlator ×N   spx-agent ×N│       │
  │   │  spx-soar   spx-scheduler   secplexus-web                │        │
  │   │            横向扩展：docker compose --scale correlator=3   │        │
  │   └───────────────────────────────────────────────────────────┘        │
  │        │             │              │             │                    │
  │   ┌────▼─────┐  ┌────▼────┐  ┌─────▼─────┐  ┌────▼──────────┐          │
  │   │ postgres │  │ redis   │  │ minio     │  │ agentgateway  │          │
  │   │ profiles:│  │         │  │ profiles: │  │ (guard 档)    │          │
  │   │[postgres]│  │         │  │  [minio]  │  │               │          │
  │   └──────────┘  └─────────┘  └───────────┘  └───────────────┘          │
  │                                                                     │
  │   .env 开关：SPX_DB_URL / SPX_REDIS_URL 填了 → 跳过捆绑容器            │
  │            SPX_OBJECT_DRIVER = fs(默认) | s3 | minio                  │
  │            profiles 分档：core（默认）→ guard → full                    │
  └─────────────────────────────────────────────────────────────────────┘

═══ 形态 C：k8s（helm）══════════════════ 形态 E：离线包（信创）════════════

  helm install secplexus                  spxctl install --offline
  每角色一个 Deployment + HPA               ├ images-amd64.tar / arm64.tar
   correlator HPA 按 tenant:entity 分区      ├ charts/ + compose/
   agent HPA 按队列积压                      └ 离线脚本 + golden path
  PDB + values-ha.yaml 高可用档              docker load 起栈
  基础组件：postgresql.enabled=true（子chart）   麒麟 / UOS / 欧拉
            或 =false + externalDatabase.*      鲲鹏/飞腾(arm64)·海光/兆芯(amd64)
            （existingSecret，不内联明文）

══════════ 统一管理层：spxctl（全部形态同一个 CLI）════════════════════════

  spxctl install | upgrade（迁移+健康检查）| start | stop | status
                 | backup（pg_dump + 对象同步）| uninstall
  安装源双通道：install.sh → GitHub（海外）/ gitee + 阿里云镜像（国内加速）
```

## 4. 基础组件：捆绑默认，可切外部

这是 GitLab Omnibus 验证了十年的模式【官方文档】：默认把 PG/Redis 捆绑在安装里——装完即用零外部依赖；要换自己的，每组件一个开关：

```ruby
# GitLab 的做法（gitlab.rb）
postgresql['enable'] = false            # 关掉捆绑的
gitlab_rails['db_host'] = '你的RDS'      # 指向外部的
```

k8s 侧是 Helm 生态事实标准【官方文档·Artifactory/Airflow/Keycloak/Kong 同款】：`postgresql.enabled=false` + `externalDatabase.host` + `existingSecret`。

**我们的核心原则（GitLab 模式的精髓）：应用永远只看连接串，bundled 还是 external 是编排层的事——app 代码完全无感知。**

### 可切换配置中心：`docker/.env` 一个文件（学 RAGFlow/Dify）

```bash
# ── 密钥（首次安装自动生成写入 .env；轮换：新密钥生效、旧密钥仅解密存量）──
SPX_SECRET_KEY=                # 主密钥：会话签名 + TOTP secret 加密（AES-GCM 的 KEK 经 HKDF 派生）

# ── 角色 ──
SPX_ROLES=all

# ── 数据库（v1 仅 PG；机制预留，见下）──
COMPOSE_PROFILES=postgresql        # 学 Dify：未来支持 MySQL = 加一个 profile 词
SPX_DB_URL=                        # 填了就跳过捆绑 pg 容器（external）

# ── 对象存储 ──
SPX_OBJECT_DRIVER=fs               # fs（默认，零依赖）| s3 | minio(捆绑)
SPX_OBJECT_ENDPOINT=               # s3 模式：OSS/COS/OBS/TOS/Ceph/自建 全兼容

# ── 队列 ──
SPX_REDIS_URL=                     # 空 = 用捆绑 redis 容器
```

对象存储三个驱动的定位：**fs** 默认（单机零依赖，Grafana 默认 SQLite 的同款哲学）；**s3** 直连用户的云存储（S3 兼容协议通吃全部主流对象存储）；**minio** compose 捆捆一个 MinIO 容器给没有云存储的用户。

数据库 v1 只支持 PG（数据面要求高不将就），但用 Dify 的 profile 互斥机制预留扩展：`db_postgres` 容器声明 `profiles: [postgresql]`，未来加 MySQL = 加一个 `db_mysql` 服务 + 方言测试，业务代码不动。

## 5. 生命周期：spxctl（学 JumpServer jmsctl）

JumpServer（32k★，国产安全）的 installer 仓库提供了 `jmsctl` CLI 管理 install/upgrade/start/stop/status/backup/uninstall——比裸 `docker compose` 友好一个量级。我们同款：

| 命令 | 行为 |
|---|---|
| `spxctl install` | 交互式/配置文件驱动起栈，**装完跑 golden path**（推一条测试告警读到研判结论才退出 0——“容器起来了”不等于“它能用”，AiSOC install.sh 同款验收纪律） |
| `spxctl upgrade` | 拉新镜像 + Alembic 迁移 + 健康检查（升级是一等公民，RAGFlow 的 `migration.sh` / Sentry self-hosted 的 upgrade.sh 同款） |
| `spxctl start/stop/status/down` | 生命周期 |
| `spxctl backup` | pg_dump + 对象存储同步 |
| `spxctl uninstall` | 清理 |

源码部署和容器部署用同一个 spxctl（命令后端检测部署形态）。

## 6. 信创与多架构

**CPU 映射**：鲲鹏/飞腾 = arm64；海光/兆芯 = amd64；龙芯 = loong64（独立指令集）；申威 sw64 不支持。

| 架构 | 支持级别 | 依据 |
|---|---|---|
| amd64 + arm64 | ✅ 开源 CI 主线 | `matrix.platform` buildx 构建合并 manifest（Dify/RAGFlow 同款）；Python 镜像双架构无障碍，原生依赖（psycopg/cryptography/orjson）双架构 wheel 齐全；agentgateway（Rust）官方双架构 release |
| loong64 | ⚠️ best-effort，不进默认 CI | 官方 Docker Hub 对 LoongArch 支持有限、Python 原生 wheel 缺失风险高；路径 = 社区构建 + 源码部署兜底 |

**信创 OS（麒麟 V10 / 统信 UOS / openEuler）**：都是标准 Linux，二进制离线装 Docker 后标准 slim 镜像可跑（鲲鹏+麒麟+Docker 组合有大量生产验证）。

**内网离线交付**（信创用户硬约束）：

```
make release-offline
  → artifacts/
      images-amd64.tar / images-arm64.tar     # docker save 全套镜像
      charts/ + compose/                      # 离线编排文件
      install-offline.sh                      # docker load + 部署 + golden path
```

内网有条件的用户用 Harbor 做内网镜像源（鲲鹏+麒麟+Harbor 有成熟实践）。

**国内安装加速**（学 JumpServer 的 `cn-quick_start.sh`）：安装脚本默认走 gitee 源 + 阿里云镜像加速，海外走 GitHub——一个脚本双源。

## 7. 部署形态总表

| 形态 | 入口 | 基础组件 | 扩展 | 适用 |
|---|---|---|---|---|
| 开发 | `make dev` | compose 捆绑 | — | 贡献者 |
| 源码 | `spxctl install --source` | 捆绑或 external | — | 无 docker 环境 |
| docker 单容器 | `SPX_ROLES=all` | env 指外部 | — | 小规模/POC/边缘 |
| **compose 全家桶** | `spxctl install` | **捆绑默认，.env 切 external** | `--scale` 按角色扩 | **默认形态** |
| k8s | `helm install` | 子 chart 默认 + externalDatabase | HPA/PDB | 生产集群 |
| 离线包 | `spxctl install --offline` | tar 内捆绑 | 多机拆角色 | 信创内网 |

## 8. 参照项目证据表（全部 ≥1w star，GitHub 实查）

| 项目 | Star | 我们抄了什么 |
|---|---|---|
| [Dify](https://github.com/langgenius/dify) | 159k | compose profiles 切换数据库（db_postgres/db_mysql 互斥 profile）；可选模块 profile 化；matrix.platform 多架构 CI |
| [Supabase](https://github.com/supabase/supabase) | 114k | monorepo（apps/packages）+ docker/ 独立目录 |
| [RAGFlow](https://github.com/infiniflow/ragflow) | 93k | `.env` 单文件可切换中心；COMPOSE_PROFILES 多维组合；migration.sh 升级一等公民 |
| [Appwrite](https://github.com/appwrite/appwrite) | 58k | compose + 迁移体系 |
| [Langfuse](https://github.com/langfuse/langfuse) | 36k | 单镜像多角色；**教训**：v3 拆角色后忘起 worker 导致摄取静默失败——standalone=all 模式天然免疫，compose/helm 必须带 healthcheck |
| [JumpServer](https://github.com/jumpserver/jumpserver) | 32k | spxctl 生命周期 CLI；国内 gitee 安装源；安全类国产开源的交付形态 |
| [FastGPT](https://github.com/labring/FastGPT) | 30k | deploy/templates + k8s + 版本管理 |
| [Temporal](https://github.com/temporalio/temporal) | 24k | 同镜像多服务的 auto-setup 模式 |
| [Wazuh](https://github.com/wazuh/wazuh) | 17k | 安全平台 all-in-one + 分布式双形态 |
| [GitLab Omnibus](https://docs.gitlab.com/omnibus/settings/database) | — | 捆绑/外部组件开关模式（十年验证） |
| [AiSOC](https://github.com/beenuar/AiSOC) | — | 安装即验收（golden pipeline）；helm HPA/PDB/values-ha；CORE/full 分档 |
| flocks | — | 单进程极简形态的可行性边界 |
