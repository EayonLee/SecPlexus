# SecPlexus

AI 原生安全运营平台。

[English](README.md) | [简体中文](README.zh-CN.md)

<div align="center">
  <img src="https://img.shields.io/badge/Go-1.27.3-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.27.3" />
  <img src="https://img.shields.io/badge/Gin-1.12.0-00ADD8" alt="Gin 1.12.0" />
  <img src="https://img.shields.io/badge/Next.js-16.4.3-000000?logo=nextdotjs&amp;logoColor=white" alt="Next.js 16.4.3" />
  <img src="https://img.shields.io/badge/React-19.3.1-61DAFB?logo=react&amp;logoColor=black" alt="React 19.3.1" />
  <img src="https://img.shields.io/badge/TypeScript-5.9.3-3178C6?logo=typescript&amp;logoColor=white" alt="TypeScript 5.9.3" />
  <img src="https://img.shields.io/badge/Tailwind_CSS-4.3.3-06B6D4?logo=tailwindcss&amp;logoColor=white" alt="Tailwind CSS 4.3.3" />
  <img src="https://img.shields.io/badge/GORM-1.31.2-EF4444" alt="GORM 1.31.2" />
  <a href="https://github.com/EayonLee/SecPlexus/stargazers">
    <img src="https://img.shields.io/github/stars/EayonLee/SecPlexus?style=flat&amp;logo=github" alt="SecPlexus GitHub 星标数" />
  </a>
</div>

## 本地开发

准备Go 1.27.3、Node.js 22.12+（建议24）和pnpm 10.32.1。

```sh
cp .env.example .env
pnpm certs:generate
# 填写.env中的数据库/初始密码、CSRF密钥与内部凭证。
# 可以连接已有PG/Redis，也可以显式部署本项目的基础组件：
pnpm compose:up postgres redis
make dev
```

`make dev`启动IAM、Console与Web并支持热更新，只连接配置的依赖，不调用Docker。
缺少`.env`时会从示例生成并提示检查后重试，不覆盖已有文件。

```sh
make build       # 构建两个Go服务及Web
pnpm preview     # 启动后端并预览Web产物；公共origin须与实际地址一致
pnpm compose:up  # 显式构建并部署整套服务
make stop        # 停止本工作区应用与容器，保留数据
```

IAM已提供真实登录、主动改密、可撤销会话、组织、自定义角色和审计。首位管理员默认是`admin`，
初始密码默认 `secplexus`，仅在空账号库使用，可通过环境配置修改。管理中心和个人安全页已接入真实API。
Console当前提供组织访问与SSE鉴权，还没有业务工作流；Web基于Next.js/shadcn，仅保留`/dashboard`的CRM V1安全智驾工作台和`/login`、`/account`、`/management`真实IAM页面；其他模板页面及菜单已删除，工作台当前展示示例数据；Web通过next-intl支持英文与简体中文切换。

启动方式、配置优先级和维护步骤见[配置指南](docs/configuration.md)。
测试入口见[测试指南](docs/testing.md)，模块设计见[IAM README](src/apps/iam/README.md)与
[Web README](src/apps/web/README.md)。

Go模块使用普通结构化`config.yaml`，通过Viper/mapstructure显式绑定环境并严格解码，命令入口采用Cobra。
本机轮转JSON日志统一放在根`logs/`；容器默认stdout。

## 项目结构

```text
src/apps/       应用代码（web、iam、console）
src/packages/   共享库，按需添加
src/contracts/  接口契约
deploy/         部署配置
scripts/        开发与发布脚本
docs/           项目文档
```

## 参与贡献

开发环境、架构边界、测试要求与 PR 流程见[贡献指南](CONTRIBUTING.zh-CN.md)。

## 贡献者

感谢每一位为 SecPlexus 做出贡献的开发者。

<p align="center">
  <a href="https://github.com/EayonLee/SecPlexus/graphs/contributors">
    <img src="https://openomy.app/svg?repo=EayonLee/SecPlexus&amp;chart=bubble" alt="SecPlexus 社区贡献统计" width="100%" />
  </a>
</p>

## 鸣谢

感谢这些项目的开发者和维护者。

<p>
  <a href="https://github.com/arhamkhnz/next-shadcn-admin-dashboard" title="next-shadcn-admin-dashboard">
    <img src="docs/assets/arhamkhnz.png" alt="next-shadcn-admin-dashboard" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/shadcn-ui/ui" title="shadcn/ui">
    <img src="docs/assets/shadcn-ui.png" alt="shadcn/ui" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/redis/redis" title="Redis">
    <img src="docs/assets/redis.svg" alt="Redis" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/postgres/postgres" title="PostgreSQL">
    <img src="docs/assets/postgresql.png" alt="PostgreSQL" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/BerriAI/litellm" title="LiteLLM">
    <img src="docs/assets/litellm.svg" alt="LiteLLM" height="64" />
  </a>
</p>
