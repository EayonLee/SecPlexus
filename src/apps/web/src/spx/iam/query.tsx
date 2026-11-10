"use client";
import type { ReactNode } from "react";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import type { AuthState } from "./types";

/** queryClient 仅保存可重新获取的业务投影，不缓存凭据或持久化会话。 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: 30000, gcTime: 300000, refetchOnWindowFocus: false, refetchOnReconnect: true },
    mutations: { retry: false, gcTime: 0 },
  },
});

export function QueryProvider({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

/** sessionScope 将账号、会话与有效权限加入所有业务查询键；权限变化必须让旧结果失效。 */
export function sessionScope(auth: AuthState) {
  return [auth.principal?.session_id, auth.principal?.auth_epoch, auth.permissions.join()] as const;
}
