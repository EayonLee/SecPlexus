# 安全例外登记

Dependabot、Code scanning 与 Secret scanning 的告警原则上必须修复。确实无法修复或经确认不可达的，
按 GitHub 支持的 dismiss 流程处理，并在本文件登记：没有登记的例外视为未处理。

登记要求：来源告警编号、判定依据（可复现的事实，不是"看起来没问题"）、补偿措施、复审日期。
复审日期到期前必须重新判断；依赖没有新版本就继续延期，有版本就升级并删除本条。

## 已登记

### GHSA-82ff-hg59-8x73 · gorilla/csrf TrustedOrigins 校验不足

- **状态**：dismissed（vulnerable code not used）
- **依赖**：`github.com/gorilla/csrf v1.7.3`（上游最新，无修复版本）
- **不可达依据**：该缺陷仅在应用把主机写入 `csrf.TrustedOrigins` 时成立。IAM 从未传入该选项
  （见 `internal/middleware/security.go` 的 `csrf.Protect` 调用），并且对全部非安全方法**先**执行自己的
  Origin 白名单校验，不在白名单内直接返回 `iam.origin_invalid`；CSRF 令牌校验使用的是逐请求签名 Cookie + 请求头，
  不依赖 TrustedOrigins。
- **补偿措施**：`internal/middleware/security_test.go` 的 `TestOriginAndCSRFBoundary` 覆盖缺失来源、
  非白名单来源与回环别名三类边界。
- **后续**：上游若发布修复版本立即升级；若要迁移到 `net/http.CrossOriginProtection`（Go 1.25+）或
  `filippo.io/csrf`，需先评估语义变化（该替代方案**忽略令牌**、且放行不带 Fetch 元数据的非浏览器请求），
  属于安全架构决策，不能作为告警清理顺带完成。
- **复审日期**：2027-03-31

### GHSA-gcqx-w48m-w2h4 · deepmerge 原型污染

- **状态**：dismissed（tolerable risk）
- **依赖**：`deepmerge 4.3.1`，经 `shadcn` 主题包传递引入（devDependency）
- **依据**：该依赖只在构建期（`shadcn/tailwind.css` 由 `globals.css` 引入）与开发者交互式 CLI 中加载，
  不进入服务端运行时；触发需要攻击者控制被合并的对象键，本项目没有此类输入路径。
- **补偿措施**：`osv-scanner.toml` 保留同名忽略条目（含到期日），CI 的 `security` job 仍会拦截新增漏洞。
- **后续**：上游发布修复版本后删除忽略条目并升级。
- **复审日期**：2027-03-31
