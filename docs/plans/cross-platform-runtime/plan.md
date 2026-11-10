# 跨平台运行时实施计划

## 方案

1. 新增 `scripts/runtime.mjs`：原子读写 `.spx/run.json`，校验 PID，提供存活检查与清理。
2. `scripts/start.mjs`：启动每个服务根时登记 PID/模块/Windows 映像名；退出时清理；启动前拒绝仍存活的重复栈；任一服务退出时按进程树清理。
3. `scripts/stop.mjs`：先按登记停止进程树；POSIX 无登记时保留监听器回退（Linux 用 `/proc`，macOS 用 `lsof`）；Windows 无登记时读取 Next 开发锁；Docker 容器停止逻辑不变。
4. `scripts/processes.mjs`：Windows 使用 `taskkill /T /F`，POSIX 使用进程组；新增真实父子进程回归测试。
5. `scripts/build.mjs` 与 `scripts/build.test.mjs`：Windows 统一经 shell 解析 pnpm `.cmd`；测试按平台生成 `.cmd`/shebang 夹具。
6. `scripts/test-scripts.mjs` + `package.json`：用 Node 枚举测试文件，消除 shell 通配符差异；新增 `pnpm stop`。
7. `Makefile`：`dev` 直接调用 Node；删除仅 POSIX 的 `scripts/dev/start.sh`；移除仅 POSIX 生效的 `GOTOOLCHAIN=` 前缀（`go.mod` 已声明 toolchain）。
8. Web E2E：Windows 二进制后缀、pnpm shell、进程树清理；Windows 无 SIGTERM 的优雅退出断言按平台分支。
9. 文档：README 与配置/测试指南改为跨平台主入口，说明 `.spx/run.json` 与可选 Make。

## 验证证据

- `make test`（Windows，2026-11-10）：脚本 39 通过/1 跳过（POSIX 专属监听器用例）、Go 全部包通过、Web 18 文件 58 用例通过。
- `make build`（Windows）：Go 两个服务与 Next 生产构建成功。
- `make dev` 全栈（Windows）：`/login` 与 `/api/v1/iam/auth/context` 均返回 200；登记 IAM、Console、Web 三个根进程。
- `make stop`：三个服务树停止，5173/8080/8081 释放，`.spx/run.json` 删除。
- `make dev web` 后 `make stop`：Next 子进程树停止，5173 释放。
- 存活登记存在时再次启动：启动前拒绝并给出 PID。
- 进程树回归：`cmd.exe` 包装层 + 孙进程在终止后均不存在。
- 开发库已按 flat-authorization 计划重建为 `00001_accounts.sql` 单一基线（Goose 1），旧 `permissions.module`/`user_sessions.user_agent` 遗留列不再存在；IAM seed 与登录通过。

## 未执行与边界

- 本机为 Windows，未实际运行 macOS/Linux；对应路径使用 Node 标准 API、POSIX 进程组与既有 `ps`/`lsof` 回退，需在 CI/各平台补跑。
- E2E 需要隔离 PostgreSQL/Redis 或 Docker；本次仅做语法检查与代码路径修正，未执行完整浏览器套件。
- Windows 上 Node 无 `SIGTERM`/`SIGBREAK` 优雅转发，E2E 仅在该平台放宽退出码断言，SSE 关闭仍被验证。
- 开发库已按新基线重建；本机完整 `make dev`、`make stop`、`make test`、`make build` 均通过。
