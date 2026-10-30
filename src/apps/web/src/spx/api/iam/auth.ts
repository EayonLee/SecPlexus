import { z } from "zod";

import { request } from "../client";
import {
  zApicsrfContext,
  zApiLoginResponse,
  zApiMe,
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
export const changePassword = (body: z.infer<typeof zChangePasswordBody>) =>
  request({ method: "POST", url: "/api/v1/iam/me/password", data: body }, z.void(), 204);
export const me = (orgId?: string, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/me", params: orgId ? { org_id: orgId } : undefined, signal }, zApiMe);
