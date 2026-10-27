# SecPlexus

AI 原生安全运营平台。

[English](README.md) | [简体中文](README.zh-CN.md)

## 本地开发

准备好 Node.js 22.12+ 和 pnpm 10.32.1，在项目根目录运行：

```sh
make dev
```

首次运行会安装依赖，然后启动前端。打开 <http://127.0.0.1:5173> 即可查看，
修改代码后页面会自动更新。按 Ctrl+C 停止。Node.js 建议使用 24 版本。

目前先做前端，IAM 和 Console 还没有实现。页面数据和登录功能都是演示用的，
账号为 `admin` / `admin`，请勿输入真实密码。

```sh
make build   # 构建前端
make stop    # 停止正在运行的项目服务
```

更多说明见[开发文档](docs/development/README.md)和
[前端 README](src/apps/web/README.md)。

## 项目结构

```text
src/apps/       应用代码（web、iam、console）
src/packages/   共享库，按需添加
src/contracts/  接口契约
deploy/         部署配置
scripts/        开发与发布脚本
docs/           项目文档
```

## 鸣谢

感谢这些项目的开发者和维护者。

<p>
  <a href="https://github.com/arco-design/arco-design" title="Arco Design">
    <img src="docs/assets/arco-design.png" alt="Arco Design" height="64" />
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
