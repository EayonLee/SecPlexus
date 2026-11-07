import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin();

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactCompiler: true,
  agentRules: false,
  // 开发模式下 Next 默认只放行 localhost 的 HMR 与静态资源请求。
  // 用 127.0.0.1 或局域网 IP 打开页面时，脚本请求会被 403 拒绝，
  // 页面只剩服务端渲染的 HTML，React 不会接管，表现为输入框无法输入。
  allowedDevOrigins: ["127.0.0.1", "localhost"],
};

export default withNextIntl(nextConfig);
