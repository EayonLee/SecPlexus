"use client";

import { AxiosHeaders } from "axios";
import { create } from "zustand";

import { timeZonePreferenceCookieValue } from "@/i18n/timezone-cookie";
import { APIError, http } from "@/spx/api/client";
import {
  authContext,
  reauthenticate as confirmPassword,
  login,
  logout,
  me,
  changePassword as updatePassword,
} from "@/spx/api/iam/auth";

import type messages from "../../../messages/en.json";
import { sessionScope, queryClient } from "./query";
import { advanceRequestScope, assertRequestScope, requestScope } from "./request-scope";
import { type AuthState, initialAuth } from "./types";

export const useIdentity = create<AuthState>(() => initialAuth);
let sessionRequest = 0;
let csrfToken = "";
let csrfPending: Promise<string> | undefined;
let channel: BroadcastChannel | undefined;

/** syncTimeZonePreferenceCookie 让服务端渲染期的时区镜像与已保存偏好一致。 */
function syncTimeZonePreferenceCookie(value: string | null | undefined) {
  if (typeof document === "undefined") return;

  // biome-ignore lint/suspicious/noDocumentCookie: 服务端渲染期需要该镜像，偏好真源仍在数据库。
  document.cookie = timeZonePreferenceCookieValue(value);
}

/** advance 取消旧范围请求和投影，不改变尚未失效的服务端会话。 */
function advance() {
  advanceRequestScope();
  void queryClient.cancelQueries();
  queryClient.clear();
  csrfToken = "";
  csrfPending = undefined;
}

function clear(status: AuthState["status"] = "anonymous") {
  advance();
  syncTimeZonePreferenceCookie(null);
  useIdentity.setState({ ...initialAuth, status }, true);
}

/** acceptIdentity 是服务端会话投影的唯一安装点，范围变化同时清理旧请求与缓存。 */
function acceptIdentity(data: AuthState) {
  const previous = useIdentity.getState();
  if (
    previous.principal &&
    JSON.stringify(sessionScope(previous)) !== JSON.stringify(sessionScope(data))
  )
    advance();
  syncTimeZonePreferenceCookie(data.timezone);
  useIdentity.setState(data, true);
}

/** initializeSession 接收跨标签会话变更通知，并加载服务端会话投影。 */
export function initializeSession() {
  if (!channel && typeof BroadcastChannel !== "undefined") {
    channel = new BroadcastChannel("spx-session");
    channel.addEventListener("message", ({ data }) => {
      if (data !== "session_changed") return;
      clear("unknown");
      void refreshMe();
    });
  }
  return refreshMe();
}

async function csrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  if (!csrfPending) {
    const started = requestScope().version;
    csrfPending = authContext()
      .then((value) => {
        if (started !== requestScope().version) throw new DOMException("Identity changed", "AbortError");
        csrfToken = value.csrf_token;
        return csrfToken;
      })
      .finally(() => {
        if (started === requestScope().version) csrfPending = undefined;
      });
  }
  return csrfPending;
}

http.interceptors.request.use(async (config) => {
  const started = requestScope().version;
  if (!["GET", "HEAD", "OPTIONS"].includes((config.method ?? "GET").toUpperCase())) {
    const headers = AxiosHeaders.from(config.headers);
    headers.set("X-CSRF-Token", await csrf());
    config.headers = headers;
  }
  assertRequestScope(started);
  return config;
});
http.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (!(error instanceof APIError)) throw error;
    const path = new URL(error.path || "/", typeof window === "undefined" ? "http://localhost" : window.location.origin)
      .pathname;
    if (error.status === 401 && path !== "/api/v1/iam/auth/login") clear();
    if (error.status === 503) useIdentity.setState({ status: "unavailable" });
    if (error.code === "iam.csrf_invalid") csrfToken = "";
    if (error.code === "permission_denied" && !path.startsWith("/api/v1/iam/me")) void refreshMe();
    throw error;
  },
);

/** refreshMe 只接受当前会话周期的最新结果，隔离并发刷新。 */
export async function refreshMe() {
  const started = requestScope().version;
  const request = ++sessionRequest;
  try {
    const data = await me();
    if (started === requestScope().version && request === sessionRequest)
      acceptIdentity({ ...data, status: "authenticated" });
  } catch (error) {
    if (started !== requestScope().version || (error instanceof DOMException && error.name === "AbortError")) return;
    // 401 的含义是"未登录"，必须落到 anonymous；若停留在 unknown，登录页会把表单保持禁用，用户无法输入。
    if (error instanceof APIError && error.status === 401) {
      if (started === requestScope().version && request === sessionRequest) clear("anonymous");
      return;
    }
    if (started === requestScope().version && request === sessionRequest)
      useIdentity.setState({ status: "unavailable" });
  }
}

/** signIn 登录成功后重新加载会话投影，并通知其他标签页。 */
export async function signIn(username: string, password: string) {
  await login({ username, password });
  clear("unknown");
  channel?.postMessage("session_changed");
  await refreshMe();
}

/** signOut 先请求服务端撤销，再清理当前标签的会话投影。 */
export async function signOut() {
  await logout();
  clear();
  channel?.postMessage("session_changed");
}

/** changePassword 成功后清理旧会话，不记录新旧密码。 */
export async function changePassword(old_password: string, new_password: string) {
  await updatePassword({ old_password, new_password });
  clear();
  channel?.postMessage("session_changed");
}

/** reauthenticate 确认当前账号密码，再刷新权限与安全凭据。 */
export async function reauthenticate(password: string) {
  advance();
  await confirmPassword({ password });
  // 同一身份确认只替换会话投影，保持当前页面挂载以保留表单草稿。
  await refreshMe();
  channel?.postMessage("session_changed");
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
    case "iam.builtin_role_immutable":
      return "builtinRoleImmutable";
    case "iam.configured_user_immutable":
      return "configuredUserImmutable";
    case "iam.last_super_admin":
      return "lastSuperAdmin";
    case "iam.role_in_use":
      return "roleInUse";
    case "iam.role_name_taken":
      return "roleNameTaken";
    case "iam.self_target_forbidden":
      return "selfTargetForbidden";
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
