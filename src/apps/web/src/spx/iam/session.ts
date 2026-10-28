"use client";

import { z } from "zod";
import { create } from "zustand";

import type { components } from "@/auth/api.gen";
import { resultCodes, resultStatuses } from "@/auth/result.gen";

import type messages from "../../../messages/en.json";
import { organizationFromPath } from "./navigation";
import { type AuthState, type Identity, initialAuth } from "./types";

export const useIdentity = create<AuthState>(() => initialAuth);
const envelope = z.object({
  code: z.enum(Object.values(resultCodes)),
  message: z.string(),
  data: z.unknown(),
  request_id: z.string(),
});
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

/** decode 在统一边界解析 Result，页面不自行拆响应信封。 */
async function decode<T>(response: Response): Promise<T> {
  if (response.status === 204) return undefined as T;
  const result = envelope.parse(await response.json());
  if (response.ok && result.code === "ok") return result.data as T;
  if (response.ok || resultStatuses[result.code] !== response.status) throw new Error("Invalid API response");
  throw new APIError(response.status, result.code, result.request_id);
}

/** csrf 只复用当前身份周期内的安全凭据，不持久化认证材料。 */
async function csrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  if (!csrfPending) {
    const started = epoch;
    csrfPending = fetch("/api/v1/iam/auth/context", {
      signal: controller.signal,
      credentials: "same-origin",
      cache: "no-store",
    })
      .then((response) => decode<components["schemas"]["APICSRFContext"]>(response))
      .then((context) => {
        if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
        csrfToken = context.csrf_token;
        return csrfToken;
      })
      .finally(() => {
        if (started === epoch) csrfPending = undefined;
      });
  }
  return csrfPending;
}

/** api 统一请求、协议错误与身份失效处理，不在日志中输出请求参数。 */
export async function api<T>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const started = epoch;
  const method = options.method ?? "GET";
  const headers = new Headers({ Accept: "application/json" });
  if (options.body !== undefined) headers.set("Content-Type", "application/json");
  let phase = "network";
  let status = 0;
  let requestID = "";
  try {
    if (!["GET", "HEAD", "OPTIONS"].includes(method)) headers.set("X-CSRF-Token", await csrf());
    if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
    const signal = AbortSignal.any([
      controller.signal,
      AbortSignal.timeout(15000),
      ...(options.signal ? [options.signal] : []),
    ]);
    const response = await fetch(`/api/v1${path}`, {
      method,
      headers,
      signal,
      credentials: "same-origin",
      cache: "no-store",
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
    if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
    phase = "decode_result";
    status = response.status;
    requestID = z.uuid().safeParse(response.headers.get("x-request-id")).data ?? "";
    try {
      return await decode<T>(response);
    } catch (error) {
      if (!(error instanceof APIError) && [401, 503].includes(status)) {
        // 代理返回非 JSON 时仍按 HTTP 状态失效身份，不能保留过期的登录状态。
        console.error(
          JSON.stringify({
            time: new Date().toISOString(),
            level: "ERROR",
            service: "web",
            layer: "browser",
            event: "api.protocol_failed",
            phase,
            status,
            request_id: requestID,
          }),
        );
        throw new APIError(status, status === 401 ? "unauthenticated" : "service_unavailable", requestID);
      }
      throw error;
    }
  } catch (error) {
    if (started !== epoch) throw new DOMException("Identity changed", "AbortError");
    if (error instanceof APIError) {
      if (error.status === 401 && path !== "/iam/auth/login") clear();
      if (error.status === 503) useIdentity.setState({ status: "unavailable" });
      if (error.code === "iam.csrf_invalid") csrfToken = "";
      if (error.code === "permission_denied" && !path.startsWith("/iam/me")) void refreshIdentity();
    } else if (!(error instanceof DOMException && error.name === "AbortError")) {
      // 不输出 Error、fetch 参数或响应正文，避免泄露连接及身份数据。
      console.error(
        JSON.stringify({
          time: new Date().toISOString(),
          level: "ERROR",
          service: "web",
          layer: "browser",
          event: "api.failed",
          phase,
          status,
          request_id: requestID,
        }),
      );
    }
    throw error;
  }
}

/** refreshIdentity 只接受当前身份周期的最新结果，隔离组织切换的并发响应。 */
export async function refreshIdentity(
  requestedOrg = organizationFromPath(window.location.pathname, window.location.search),
) {
  let orgID = requestedOrg;
  const started = epoch;
  const request = ++identityRequest;
  try {
    let data = await api<Identity>(`/iam/me${orgID ? `?org_id=${encodeURIComponent(orgID)}` : ""}`);
    if (!orgID && data.organizations.length) {
      orgID = data.organizations[0].id;
      data = await api<Identity>(`/iam/me?org_id=${encodeURIComponent(orgID)}`);
    }
    if (started === epoch && request === identityRequest)
      useIdentity.setState({ ...data, status: "authenticated" }, true);
  } catch (error) {
    if (started !== epoch || (error instanceof DOMException && error.name === "AbortError")) return;
    if (error instanceof APIError && error.status === 401) return;
    if (error instanceof APIError && error.status === 403 && orgID) {
      try {
        const data = await api<Identity>("/iam/me");
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
  await api("/iam/auth/login", { method: "POST", body: { username, password } });
  clear("unknown");
  channel?.postMessage("session_changed");
  await refreshIdentity();
}

/** signOut 先请求服务端撤销，再清理当前标签的身份投影。 */
export async function signOut(all = false) {
  await api(all ? "/iam/auth/logout-all" : "/iam/auth/logout", { method: "POST" });
  clear();
  channel?.postMessage("session_changed");
}

/** changePassword 成功后清理旧会话，不记录新旧密码。 */
export async function changePassword(old_password: string, new_password: string) {
  await api("/iam/me/password", { method: "POST", body: { old_password, new_password } });
  clear();
  channel?.postMessage("session_changed");
}

/** reauthenticate 确认当前账号密码，再刷新能力与安全凭据。 */
export async function reauthenticate(password: string) {
  await api("/iam/auth/reauthenticate", { method: "POST", body: { password } });
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
