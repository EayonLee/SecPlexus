import { z } from "zod";

import { request } from "../client";
import {
  zApicsrfContext,
  zApiLoginResponse,
  zApiMe,
  zApiSession,
  type zChangePasswordBody,
  type zLoginBody,
  type zReauthenticateBody,
} from "./schemas";

export const authContext = () => request({ url: "/api/v1/iam/auth/context" }, zApicsrfContext);
export const login = (body: z.infer<typeof zLoginBody>) =>
  request({ method: "POST", url: "/api/v1/iam/auth/login", data: body }, zApiLoginResponse);
export const reauthenticate = (body: z.infer<typeof zReauthenticateBody>) =>
  request({ method: "POST", url: "/api/v1/iam/auth/reauthenticate", data: body }, zApiLoginResponse);
export const logout = () => request({ method: "POST", url: "/api/v1/iam/auth/logout" }, z.void(), 204);
export const logoutAll = () => request({ method: "POST", url: "/api/v1/iam/auth/logout-all" }, z.void(), 204);
export const changePassword = (body: z.infer<typeof zChangePasswordBody>) =>
  request({ method: "POST", url: "/api/v1/iam/me/password", data: body }, z.void(), 204);
export const me = (orgId?: string, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/me", params: orgId ? { org_id: orgId } : undefined, signal }, zApiMe);
export const listSessions = (signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/me/sessions", signal }, z.object({ items: z.array(zApiSession) }));
export const revokeSession = (id: string) =>
  request({ method: "DELETE", url: `/api/v1/iam/me/sessions/${encodeURIComponent(id)}` }, z.void(), 204);
