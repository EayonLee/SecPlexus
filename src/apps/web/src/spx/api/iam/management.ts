import { z } from "zod";

import { pageSchema, request } from "../client";
import {
  type AuditQuery,
  type ListQuery,
  zApiAuditPage,
  zApiCreated,
  zApiPermissionItem,
  zApiRole,
  zApiUser,
  type zCreateRoleBody,
  type zCreateUserBody,
  type zResetUserPasswordBody,
  type zSetRolePermissionsBody,
  type zSetUserRolesBody,
  type zUpdateRoleBody,
  type zUpdateUserBody,
} from "./schemas";

export const listUsers = (query?: ListQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/users", params: query, signal }, pageSchema(zApiUser));
export const createUser = (body: z.infer<typeof zCreateUserBody>) =>
  request({ method: "POST", url: "/api/v1/iam/users", data: body }, zApiCreated, 201);
export const updateUser = ({ id, body }: { id: string; body: z.infer<typeof zUpdateUserBody> }) =>
  request({ method: "PATCH", url: `/api/v1/iam/users/${encodeURIComponent(id)}`, data: body }, z.void(), 204);
export const deleteUser = ({ id }: { id: string }) =>
  request({ method: "DELETE", url: `/api/v1/iam/users/${encodeURIComponent(id)}` }, z.void(), 204);
export const setUserRoles = ({ id, body }: { id: string; body: z.infer<typeof zSetUserRolesBody> }) =>
  request({ method: "PUT", url: `/api/v1/iam/users/${encodeURIComponent(id)}/roles`, data: body }, z.void(), 204);
export const resetUserPassword = ({ id, body }: { id: string; body: z.infer<typeof zResetUserPasswordBody> }) =>
  request(
    { method: "POST", url: `/api/v1/iam/users/${encodeURIComponent(id)}/reset-password`, data: body },
    z.void(),
    204,
  );
export const listRoles = (query?: ListQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/roles", params: query, signal }, pageSchema(zApiRole));
export const createRole = (body: z.infer<typeof zCreateRoleBody>) =>
  request({ method: "POST", url: "/api/v1/iam/roles", data: body }, zApiCreated, 201);
export const updateRole = ({ id, body }: { id: string; body: z.infer<typeof zUpdateRoleBody> }) =>
  request({ method: "PATCH", url: `/api/v1/iam/roles/${encodeURIComponent(id)}`, data: body }, z.void(), 204);
export const deleteRole = ({ id }: { id: string }) =>
  request({ method: "DELETE", url: `/api/v1/iam/roles/${encodeURIComponent(id)}` }, z.void(), 204);
export const setRolePermissions = ({ id, body }: { id: string; body: z.infer<typeof zSetRolePermissionsBody> }) =>
  request({ method: "PUT", url: `/api/v1/iam/roles/${encodeURIComponent(id)}/permissions`, data: body }, z.void(), 204);
export const permissions = (signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/permissions", signal }, z.object({ items: z.array(zApiPermissionItem) }));
export const listAuditEvents = (query?: AuditQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/audit-events", params: query, signal }, zApiAuditPage);
