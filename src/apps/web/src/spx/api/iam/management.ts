import { z } from "zod";

import { pageSchema, request } from "../client";
import {
  type AuditQuery,
  type ListQuery,
  zApiAuditEvent,
  zApiCreated,
  zApiOrganizationUser,
  zApiOrganization,
  zApiPermissionItem,
  zApiPreview,
  zApiRole,
  zApiUser,
  zOrganizationTree,
  type zCreateOrganizationUserBody,
  type zCreateOrganizationBody,
  type zCreateRoleBody,
  type zCreateUserBody,
  type zResetUserPasswordBody,
  type zSetOrganizationUserRolesBody,
  type zSetRolePermissionsBody,
  type zUpdateOrganizationUserBody,
  type zUpdateOrganizationBody,
  type zUpdateRoleBody,
  type zUpdateUserBody,
} from "./schemas";

export const listUsers = (query?: ListQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/users", params: query, signal }, pageSchema(zApiUser));
export const createUser = (body: z.infer<typeof zCreateUserBody>) =>
  request({ method: "POST", url: "/api/v1/iam/users", data: body }, zApiCreated, 201);
export const updateUser = ({ id, body }: { id: string; body: z.infer<typeof zUpdateUserBody> }) =>
  request({ method: "PATCH", url: `/api/v1/iam/users/${encodeURIComponent(id)}`, data: body }, z.void(), 204);
export const resetUserPassword = ({ id, body }: { id: string; body: z.infer<typeof zResetUserPasswordBody> }) =>
  request(
    { method: "POST", url: `/api/v1/iam/users/${encodeURIComponent(id)}/reset-password`, data: body },
    z.void(),
    204,
  );
export const listOrganizations = (query?: ListQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/organizations", params: query, signal }, pageSchema(zApiOrganization));
export const listOrganizationTree = (signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/organizations/tree", signal }, zOrganizationTree);
export const createOrganization = (body: z.infer<typeof zCreateOrganizationBody>) =>
  request({ method: "POST", url: "/api/v1/iam/organizations", data: body }, zApiCreated, 201);
export const updateOrganization = ({ id, body }: { id: string; body: z.infer<typeof zUpdateOrganizationBody> }) =>
  request({ method: "PATCH", url: `/api/v1/iam/organizations/${encodeURIComponent(id)}`, data: body }, z.void(), 204);
export const listOrganizationUsers = (orgId: string, query?: ListQuery, signal?: AbortSignal) =>
  request(
    { url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/users`, params: query, signal },
    pageSchema(zApiOrganizationUser),
  );
export const createOrganizationUser = ({ orgId, body }: { orgId: string; body: z.infer<typeof zCreateOrganizationUserBody> }) =>
  request(
    { method: "POST", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/users`, data: body },
    zApiCreated,
    201,
  );
export const updateOrganizationUser = ({
  orgId,
  id,
  body,
}: {
  orgId: string;
  id: string;
  body: z.infer<typeof zUpdateOrganizationUserBody>;
}) =>
  request(
    {
      method: "PATCH",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/users/${encodeURIComponent(id)}`,
      data: body,
    },
    z.void(),
    204,
  );
export const deleteOrganizationUser = ({ orgId, id, version }: { orgId: string; id: string; version: string }) =>
  request(
    {
      method: "DELETE",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/users/${encodeURIComponent(id)}`,
      params: { authz_epoch: version },
    },
    z.void(),
    204,
  );
export const listRoles = (orgId: string, query?: ListQuery, signal?: AbortSignal) =>
  request({ url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles`, params: query, signal }, pageSchema(zApiRole));
export const createRole = ({ orgId, body }: { orgId: string; body: z.infer<typeof zCreateRoleBody> }) =>
  request({ method: "POST", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles`, data: body }, zApiCreated, 201);
export const updateRole = ({ orgId, id, body }: { orgId: string; id: string; body: z.infer<typeof zUpdateRoleBody> }) =>
  request(
    {
      method: "PATCH",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}`,
      data: body,
    },
    z.void(),
    204,
  );
export const deleteRole = ({ orgId, id, version }: { orgId: string; id: string; version: string }) =>
  request(
    {
      method: "DELETE",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}`,
      params: { version },
    },
    z.void(),
    204,
  );
export const setRolePermissions = ({
  orgId,
  id,
  body,
}: {
  orgId: string;
  id: string;
  body: z.infer<typeof zSetRolePermissionsBody>;
}) =>
  request(
    {
      method: "PUT",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}/permissions`,
      data: body,
    },
    z.void(),
    204,
  );
export const setOrganizationUserRoles = ({
  orgId,
  id,
  body,
}: {
  orgId: string;
  id: string;
  body: z.infer<typeof zSetOrganizationUserRolesBody>;
}) =>
  request(
    {
      method: "PUT",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/users/${encodeURIComponent(id)}/roles`,
      data: body,
    },
    z.void(),
    204,
  );
export const listRoleUsers = (orgId: string, id: string, query?: ListQuery, signal?: AbortSignal) =>
  request(
    {
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}/users`,
      params: query,
      signal,
    },
    pageSchema(zApiOrganizationUser),
  );
export const permissions = (orgId?: string, signal?: AbortSignal) =>
  request(
    { url: "/api/v1/iam/permissions", params: orgId ? { org_id: orgId } : undefined, signal },
    z.object({ items: z.array(zApiPermissionItem) }),
  );
export const listAuditEvents = (query?: AuditQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/audit-events", params: query, signal }, pageSchema(zApiAuditEvent));
