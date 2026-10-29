"use client";
import type { ReactNode } from "react";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import type { AuthState } from "./types";

/** queryClient 仅保存可重新获取的业务投影，不缓存凭据或持久化身份。 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: 30000, gcTime: 300000, refetchOnWindowFocus: false, refetchOnReconnect: true },
    mutations: { retry: false, gcTime: 0 },
  },
});

export function QueryProvider({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

/** identityScope 将账号、会话和组织授权版本加入所有业务查询键。 */
export function identityScope(auth: AuthState) {
  return [
    auth.principal?.session_id,
    auth.principal?.auth_version,
    auth.org_id,
    auth.organizations.find((org) => org.id === auth.org_id)?.authz_version,
  ] as const;
}
