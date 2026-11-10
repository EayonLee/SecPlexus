# 跨平台运行时与开发工具

## 状态

已实施（Windows 实测；macOS/Linux 由同一 Node 入口与进程组路径覆盖，未在本机实跑）。

## 问题

项目此前以 macOS 为主要开发环境，Windows 上暴露出三类问题：

1. Windows 没有 POSIX 进程组。只结束 `cmd.exe`/pnpm 直接子进程会留下 Next 或 Go 子进程，继续占用端口并触发 Next 的单实例锁。
2. `stop` 依赖 `ps`/`lsof`/`process.getuid()`，在 Windows 直接失败；Linux 最小环境也不保证有 `lsof`。
3. 开发命令依赖 `sh`、GNU Make、shell 通配符和 Unix 可执行权限，Windows PowerShell/cmd 无法等价执行。

## 行为契约

- `pnpm dev`、`pnpm preview`、`pnpm stop`、`pnpm build`、`pnpm test:unit` 在 Windows（PowerShell/cmd）、macOS、Linux 使用同一入口。
- GNU Make 是 macOS、Linux、Windows 的唯一外层入口；`make dev/build/stop/test/...` 不得依赖 POSIX shell 或跨平台不可用的环境变量前缀。
- 启动器在仓库内写 `.spx/run.json`，记录本次启动的服务根 PID、模块和 Windows 映像名；正常退出删除，异常退出保留。
- 第二次启动发现登记进程仍存活时拒绝启动并提示先停止；登记进程均已退出时清理陈旧登记后继续。若存在仍存活的 Next 开发锁，启动前同样拒绝并提示先停止。
- `stop` 优先按登记进程树停止：Windows 使用 `taskkill /T /F`，POSIX 使用进程组信号；无登记时保留 POSIX 监听器回退（Linux 读取 `/proc`，macOS 使用系统自带 `lsof`），Windows 使用 Next 开发锁回退。
- 构建和启动脚本在 Windows 通过 `cmd.exe` 解析 pnpm 的 `.cmd` shim；Go 产物使用 `.exe` 后缀。
- 测试入口不得依赖 shell 展开 `scripts/*.test.mjs`；由 Node 枚举并传给 `node --test`。
- 测试夹具按平台生成可执行 shim，不把 Unix shebang 文件当作 Windows 命令。

## 验收

- Windows 上 `pnpm test:unit` 通过（脚本、Go、Web 单测）。
- Windows 上 `pnpm build` 成功，Web 产物完整。
- Windows 上启动 Web 子栈后 `pnpm stop` 释放端口并删除登记；第二次启动能识别存活登记。
- 进程树回归测试证明终止后孙进程不存活。
- macOS/Linux 保留进程组终止路径和既有 POSIX 监听器回退；本机未实跑的部分在 Plan 中列明。

## 非目标

- 不引入平台专用的生产运行时代码或新的进程管理器依赖。
- 不改变 IAM/Console/Web 的业务契约、端口默认值或容器编排。
- 不把 Windows 的强制终止伪装成优雅退出；E2E 的优雅退出断言仅在 POSIX 成立。
