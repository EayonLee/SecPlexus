import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin();

// 安全响应头取值依据 OWASP HTTP Headers Cheat Sheet。这里只加与渲染无关的约束：
// CSP 只写 frame-ancestors/base-uri/object-src，不写 script-src——内联启动脚本
// 需要 nonce 方案，属于独立改动，写死 script-src 只会把页面打挂。
// HSTS 在明文 HTTP 下会被浏览器忽略，因此由反代终止 TLS 的部署同样能收到。
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Content-Security-Policy", value: "frame-ancestors 'none'; base-uri 'self'; object-src 'none'" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), usb=(), payment=()" },
  { key: "Strict-Transport-Security", value: "max-age=31536000" },
  { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
  { key: "X-XSS-Protection", value: "0" },
];

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactCompiler: true,
  // 不暴露框架标识：响应头里的 x-powered-by 等于把技术栈和大致版本告诉扫描器。
  poweredByHeader: false,
  agentRules: false,
  // 开发模式下 Next 默认只放行 localhost 的 HMR 与静态资源请求。
  // 用 127.0.0.1 或局域网 IP 打开页面时，脚本请求会被 403 拒绝，
  // 页面只剩服务端渲染的 HTML，React 不会接管，表现为输入框无法输入。
  allowedDevOrigins: ["127.0.0.1", "localhost"],
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

export default withNextIntl(nextConfig);
