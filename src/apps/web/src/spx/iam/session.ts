"use client";

import { z } from "zod";
import { create } from "zustand";

import { resultStatuses } from "@/auth/result.gen";
import { client as consoleClient } from "@/spx/api/console/client.gen";
import { client } from "@/spx/api/iam/client.gen";
import {
  authContext,
  reauthenticate as confirmPassword,
  login,
  logout,
  logoutAll,
  me,
  changePassword as updatePassword,
} from "@/spx/api/iam/sdk.gen";
import { zApiError } from "@/spx/api/iam/zod.gen";

import type messages from "../../../messages/en.json";
import { organizationFromPath } from "./navigation";
import { queryClient } from "./query";
import { type AuthState, initialAuth } from "./types";

export const useIdentity = create<AuthState>(() => initialAuth);
let epoch = 0;
let identityRequest = 0;
let controller = new AbortController();
let csrfToken = "";
let csrfPending: Promise<string> | undefined;
let channel: BroadcastChannel | undefined;

/** APIError 只向页面传递稳定结果码、HTTP 状态和排障标识。 */
export class APIError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly requestID = "",
  ) {
    super(code);
  }
}

/** clear 取消旧身份的请求，避免退出或切换组织后写回过期结果。 */
function clear(status: AuthState["status"] = "anonymous") {
  epoch++;
  controller.abort();
  void queryClient.cancelQueries();
  queryClient.clear();
  controller = new AbortController();
  csrfToken = "";
  csrfPending = undefined;
  useIdentity.setState({ ...initialAuth, status }, true);
}

/** initializeSession 接收跨标签身份变更通知，并加载服务端身份。 */
export function initializeSession() {
  if (!channel && typeof BroadcastChannel !== "undefined") {
    channel = new BroadcastChannel("spx-identity");
    channel.addEventListener("message", ({ data }) => {
      if (data !== "session_changed") return;
      clear("unknown");
      void refreshIdentity();
    });
  }
  return refreshIdentity();
}

/** mutationHeaders 声明生成 SDK 的必需协议头，真实安全值由请求拦截器填入。 */
export const mutationHeaders = { "X-CSRF-Token": "", Origin: "" };

/** dataOf 解开已由生成 Zod 验证的成功信封；204 无业务正文。 */
export async function dataOf<T>(operation: Promise<{ data: { data: T } }>): Promise<T> {
  const started = epoch;
  const result = await operation;
  if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
  return result.data.data;
}

async function csrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  if (!csrfPending) {
    const started = epoch;
    csrfPending = dataOf(authContext({ throwOnError: true }))
      .then((value) => {
        if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
        csrfToken = value.csrf_token;
        return csrfToken;
      })
      .finally(() => {
        if (started === epoch) csrfPending = undefined;
      });
  }
  return csrfPending;
}

const requestEpochs = new WeakMap<Request, number>();
for (const sdk of [client, consoleClient]) {
  sdk.setConfig({
    baseUrl: typeof window === "undefined" ? "" : window.location.origin,
    credentials: "same-origin",
    cache: "no-store",
    throwOnError: true,
  });
  sdk.interceptors.request.use(async (request) => {
    const started = epoch;
    const currentController = controller;
    const headers = new Headers(request.headers);
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method)) {
      headers.set("X-CSRF-Token", await csrf());
      headers.set("Origin", window.location.origin);
    }
    if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
    const next = new Request(request, {
      headers,
      signal: AbortSignal.any([request.signal, currentController.signal, AbortSignal.timeout(15000)]),
    });
    requestEpochs.set(next, started);
    return next;
  });
  sdk.interceptors.response.use(async (response, request) => {
    if (requestEpochs.get(request) !== epoch) throw new DOMException("Identity changed", "AbortError");
    if (response.ok) return response;
    let value: unknown;
    // 代理在故障时可能返回文本；仅公开状态与稳定错误码。
    const raw = await response.clone().text();
    try {
      value = JSON.parse(raw);
    } catch {
      value = undefined;
    }
    const parsed = zApiError.safeParse(value);
    let code = "internal_error";
    if (parsed.success && resultStatuses[parsed.data.code] === response.status) code = parsed.data.code;
    else if (response.status === 401) code = "unauthenticated";
    else if (response.status === 503) code = "service_unavailable";
    const error = new APIError(
      response.status,
      code,
      parsed.success ? parsed.data.request_id : (z.uuid().safeParse(response.headers.get("x-request-id")).data ?? ""),
    );
    const path = new URL(request.url).pathname;
    if (error.status === 401 && path !== "/api/v1/iam/auth/login") clear();
    if (error.status === 503) useIdentity.setState({ status: "unavailable" });
    if (error.code === "iam.csrf_invalid") csrfToken = "";
    if (error.code === "permission_denied" && !path.startsWith("/api/v1/iam/me")) void refreshIdentity();
    throw error;
  });
  sdk.interceptors.error.use((error, response, _request, options) => {
    if (error instanceof z.ZodError) {
      console.error(
        JSON.stringify({
          time: new Date().toISOString(),
          level: "ERROR",
          service: "web",
          layer: "browser",
          event: "api.protocol_failed",
          route: options.url,
          status: response?.status,
          request_id: response?.headers.get("x-request-id"),
          issues: error.issues.map((issue) => ({ code: issue.code, path: issue.path })),
        }),
      );
    }
    return error;
  });
}

/** refreshIdentity 只接受当前身份周期的最新结果，隔离组织切换的并发响应。 */
export async function refreshIdentity(
  requestedOrg = organizationFromPath(window.location.pathname, window.location.search),
) {
  let orgID = requestedOrg;
  const started = epoch;
  const request = ++identityRequest;
  try {
    let data = await dataOf(me({ query: orgID ? { org_id: orgID } : undefined, throwOnError: true }));
    if (!orgID && data.organizations.length) {
      orgID = data.organizations[0].id;
      data = await dataOf(me({ query: { org_id: orgID }, throwOnError: true }));
    }
    if (started === epoch && request === identityRequest)
      useIdentity.setState({ ...data, status: "authenticated" }, true);
  } catch (error) {
    if (started !== epoch || (error instanceof DOMException && error.name === "AbortError")) return;
    if (error instanceof APIError && error.status === 401) return;
    if (error instanceof APIError && error.status === 403 && orgID) {
      try {
        const data = await dataOf(me({ throwOnError: true }));
        if (started === epoch && request === identityRequest)
          useIdentity.setState({ ...data, org_id: orgID, capabilities: [], status: "authenticated" }, true);
      } catch {
        if (started === epoch && request === identityRequest) useIdentity.setState({ status: "unavailable" });
      }
      return;
    }
    if (started === epoch && request === identityRequest) useIdentity.setState({ status: "unavailable" });
  }
}

/** signIn 登录成功后重新加载身份，并通知其他标签页。 */
export async function signIn(username: string, password: string) {
  await login({ body: { username, password }, headers: mutationHeaders, throwOnError: true });
  clear("unknown");
  channel?.postMessage("session_changed");
  await refreshIdentity();
}

/** signOut 先请求服务端撤销，再清理当前标签的身份投影。 */
export async function signOut(all = false) {
  await (all ? logoutAll : logout)({ headers: mutationHeaders, throwOnError: true });
  clear();
  channel?.postMessage("session_changed");
}

/** changePassword 成功后清理旧会话，不记录新旧密码。 */
export async function changePassword(old_password: string, new_password: string) {
  await updatePassword({ body: { old_password, new_password }, headers: mutationHeaders, throwOnError: true });
  clear();
  channel?.postMessage("session_changed");
}

/** reauthenticate 确认当前账号密码，再刷新能力与安全凭据。 */
export async function reauthenticate(password: string) {
  await confirmPassword({ body: { password }, headers: mutationHeaders, throwOnError: true });
  csrfToken = "";
  channel?.postMessage("session_changed");
  await refreshIdentity();
}

/** switchOrganization 取消旧组织请求，再加载目标组织能力。 */
export async function switchOrganization(orgID: string) {
  clear("unknown");
  await refreshIdentity(orgID);
}

/** errorMessageKey 将稳定结果码映射为国际化消息键，不显示内部错误正文。 */
export function errorMessageKey(error: unknown): keyof (typeof messages)["Errors"] {
  if (!(error instanceof APIError)) return "generic";
  switch (error.code) {
    case "iam.invalid_current_password":
      return "invalidCurrentPassword";
    case "iam.invalid_credentials":
      return "invalidCredentials";
    case "iam.recent_auth_required":
      return "recentAuthRequired";
    case "permission_denied":
      return "permissionDenied";
    case "conflict":
      return "conflict";
    case "iam.csrf_invalid":
      return "csrfInvalid";
    case "invalid_request":
      return "invalidRequest";
    case "rate_limited":
      return "rateLimited";
    default:
      return error.status === 503 ? "unavailable" : "generic";
  }
}
