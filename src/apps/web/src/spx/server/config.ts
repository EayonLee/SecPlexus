// 服务端接入配置只在模块加载时读取，不暴露给浏览器。
const iam = new URL(process.env.SPX_WEB_IAM_PROXY_TARGET || "http://127.0.0.1:8081");
const consoleService = new URL(process.env.SPX_WEB_CONSOLE_PROXY_TARGET || "http://127.0.0.1:8080");
for (const url of [iam, consoleService]) {
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash
  )
    throw new Error("Invalid API upstream origin");
}
export const apiOrigins = { iam: iam.origin, console: consoleService.origin };
