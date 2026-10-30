import { readWebConfiguration } from "@/config/environment.mjs";

// 只在服务端加载环境，不将上游位置或认证凭据放入客户端包。
export const { apiOrigins } = readWebConfiguration(process.env);
