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
chmod 600 .env
# 填写.env中的数据库/管理员密码、CSRF密钥与服务凭证。
# 可以连接已有PG/Redis，也可以显式部署本项目的基础组件：
pnpm compose:up postgres redis
pnpm config:check
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
唯一超级管理员默认 `admin/secplexus`，身份与密码由 `SPX_IAM_BOOTSTRAP_USERNAME` / `SPX_IAM_BOOTSTRAP_PASSWORD` 持续管理，修改后重启全部 IAM 副本，不能在页面中修改。管理中心和个人设置已接入真实API。
Console当前提供组织访问与SSE鉴权，还没有业务工作流；Web基于Next.js/shadcn，仅保留`/dashboard`的CRM V1安全智驾工作台和`/login`、`/settings/profile`、`/settings/account`、`/management`真实IAM页面；其他模板页面及菜单已删除，工作台当前展示示例数据；Web通过next-intl支持英文与简体中文切换。

启动方式、配置优先级和维护步骤见[配置指南](docs/configuration.md)。
测试入口见[测试指南](docs/testing.md)，模块设计见[IAM README](src/apps/iam/README.md)与
[Web README](src/apps/web/README.md)。

Go模块在`config.yaml`中声明环境变量和默认值，使用成熟参数组件展开，再由Viper/mapstructure严格解码，命令入口采用Cobra。
本机轮转JSON日志统一放在根`logs/`；容器默认stdout。

## 项目结构

```text
src/apps/       应用代码（web、iam、console）
src/packages/   共享库，按需添加
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

感谢 SecPlexus 及其开发工具所使用项目的开发者和维护者。

<p>
  <a href="https://github.com/arhamkhnz/next-shadcn-admin-dashboard" title="next-shadcn-admin-dashboard">
    <img src="docs/assets/arhamkhnz.png" alt="next-shadcn-admin-dashboard" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/shadcn-ui/ui" title="shadcn/ui">
    <img src="docs/assets/shadcn-ui.png" alt="shadcn/ui" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/postgres/postgres" title="PostgreSQL">
    <img src="docs/assets/postgresql.png" alt="PostgreSQL" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/redis/redis" title="Redis">
    <img src="docs/assets/redis.svg" alt="Redis" height="64" />
  </a>
</p>

<p>
  <a href="https://github.com/gin-gonic/gin" title="Gin">
    <img src="docs/assets/gin.svg" alt="Gin" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/go-gorm/gorm" title="GORM">
    <img src="docs/assets/gorm.svg" alt="GORM" width="120" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/spf13/viper" title="Viper">
    <img src="docs/assets/viper.png" alt="Viper" width="120" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/spf13/cobra" title="Cobra">
    <img src="docs/assets/cobra.png" alt="Cobra" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/pressly/goose" title="Goose">
    <img src="docs/assets/goose.png" alt="Goose" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/gorilla/csrf" title="Gorilla CSRF">
    <img src="docs/assets/gorilla.png" alt="Gorilla CSRF" height="56" />
  </a>
</p>

<p>
  <a href="https://github.com/facebook/react" title="React">
    <img src="docs/assets/react.svg" alt="React" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/vercel/next.js" title="Next.js">
    <img src="docs/assets/nextjs.png" alt="Next.js" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/tailwindlabs/tailwindcss" title="Tailwind CSS">
    <img src="docs/assets/tailwind-mark.svg" alt="Tailwind CSS" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/radix-ui/primitives" title="Radix UI">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/radix-ui-dark.svg" />
      <img src="docs/assets/radix-ui.svg" alt="Radix UI" height="56" />
    </picture>
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/mui/base-ui" title="Base UI">
    <img src="docs/assets/base-ui.png" alt="Base UI" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/lucide-icons/lucide" title="Lucide">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/lucide-dark.svg" />
      <img src="docs/assets/lucide.svg" alt="Lucide" width="120" />
    </picture>
  </a>
</p>

<p>
  <a href="https://tanstack.com" title="TanStack Query / Table">
    <img src="docs/assets/tanstack-query.svg" alt="TanStack Query / Table" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/axios/axios" title="Axios">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/axios-dark.svg" />
      <img src="docs/assets/axios.svg" alt="Axios" width="120" />
    </picture>
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/colinhacks/zod" title="Zod">
    <img src="docs/assets/zod.svg" alt="Zod" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/react-hook-form/react-hook-form" title="React Hook Form">
    <img src="docs/assets/react-hook-form.svg" alt="React Hook Form" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/amannn/next-intl" title="next-intl">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="docs/assets/next-intl-dark.svg" />
      <img src="docs/assets/next-intl.svg" alt="next-intl" width="180" />
    </picture>
  </a>
</p>

<p>
  <a href="https://github.com/microsoft/playwright" title="Playwright">
    <img src="docs/assets/playwright.svg" alt="Playwright" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/vitest-dev/vitest" title="Vitest">
    <img src="docs/assets/vitest.svg" alt="Vitest" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/biomejs/biome" title="Biome">
    <img src="docs/assets/biome.svg" alt="Biome" height="56" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/peteromallet/desloppify" title="Desloppify">
    <img src="docs/assets/desloppify.png" alt="Desloppify" height="56" />
  </a>
</p>
