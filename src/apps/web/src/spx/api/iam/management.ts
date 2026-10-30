import { z } from "zod";

import { pageSchema, request } from "../client";
import {
  type ListQuery,
  zApiAuditEvent,
  zApiCreated,
  zApiMember,
  zApiOrganization,
  zApiPermissionItem,
  zApiPreview,
  zApiRole,
  zApiUser,
  type zCreateMemberBody,
  type zCreateOrganizationBody,
  type zCreateRoleBody,
  type zCreateUserBody,
  type zPreviewPermissionBody,
  type zResetUserPasswordBody,
  type zSetMemberRolesBody,
  type zSetRolePermissionsBody,
  type zUpdateCurrentOrganizationBody,
  type zUpdateMemberBody,
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
export const createOrganization = (body: z.infer<typeof zCreateOrganizationBody>) =>
  request({ method: "POST", url: "/api/v1/iam/organizations", data: body }, zApiCreated, 201);
export const updateOrganization = ({ id, body }: { id: string; body: z.infer<typeof zUpdateOrganizationBody> }) =>
  request({ method: "PATCH", url: `/api/v1/iam/organizations/${encodeURIComponent(id)}`, data: body }, z.void(), 204);
export const getOrganization = (orgId: string, signal?: AbortSignal) =>
  request({ url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}`, signal }, zApiOrganization);
export const updateCurrentOrganization = ({
  orgId,
  body,
}: {
  orgId: string;
  body: z.infer<typeof zUpdateCurrentOrganizationBody>;
}) => request({ method: "PATCH", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}`, data: body }, z.void(), 204);
export const listMembers = (orgId: string, query?: ListQuery, signal?: AbortSignal) =>
  request(
    { url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/members`, params: query, signal },
    pageSchema(zApiMember),
  );
export const createMember = ({ orgId, body }: { orgId: string; body: z.infer<typeof zCreateMemberBody> }) =>
  request(
    { method: "POST", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/members`, data: body },
    zApiCreated,
    201,
  );
export const updateMember = ({
  orgId,
  id,
  body,
}: {
  orgId: string;
  id: string;
  body: z.infer<typeof zUpdateMemberBody>;
}) =>
  request(
    {
      method: "PATCH",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/members/${encodeURIComponent(id)}`,
      data: body,
    },
    z.void(),
    204,
  );
export const deleteMember = ({ orgId, id, version }: { orgId: string; id: string; version: string }) =>
  request(
    {
      method: "DELETE",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/members/${encodeURIComponent(id)}`,
      params: { authz_version: version },
    },
    z.void(),
    204,
  );
export const listRoles = (orgId: string, query?: ListQuery, signal?: AbortSignal) =>
  request({ url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles`, params: query, signal }, pageSchema(zApiRole));
export const createRole = ({ orgId, body }: { orgId: string; body: z.infer<typeof zCreateRoleBody> }) =>
  request({ method: "POST", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles`, data: body }, zApiCreated, 201);
export const getRole = (orgId: string, id: string, signal?: AbortSignal) =>
  request({ url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}`, signal }, zApiRole);
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
export const setMemberRoles = ({
  orgId,
  id,
  body,
}: {
  orgId: string;
  id: string;
  body: z.infer<typeof zSetMemberRolesBody>;
}) =>
  request(
    {
      method: "PUT",
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/members/${encodeURIComponent(id)}/roles`,
      data: body,
    },
    z.void(),
    204,
  );
export const listRoleMembers = (orgId: string, id: string, query?: ListQuery, signal?: AbortSignal) =>
  request(
    {
      url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(id)}/members`,
      params: query,
      signal,
    },
    pageSchema(zApiMember),
  );
export const previewPermission = ({ orgId, body }: { orgId: string; body: z.infer<typeof zPreviewPermissionBody> }) =>
  request(
    { method: "POST", url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/permission-preview`, data: body },
    zApiPreview,
  );
export const permissions = (orgId?: string, signal?: AbortSignal) =>
  request(
    { url: "/api/v1/iam/permissions", params: orgId ? { org_id: orgId } : undefined, signal },
    z.object({ items: z.array(zApiPermissionItem) }),
  );
export const listAuditEvents = (query?: ListQuery, signal?: AbortSignal) =>
  request({ url: "/api/v1/iam/audit-events", params: query, signal }, pageSchema(zApiAuditEvent));
export const listOrganizationAuditEvents = (orgId: string, query?: ListQuery, signal?: AbortSignal) =>
  request(
    { url: `/api/v1/iam/orgs/${encodeURIComponent(orgId)}/audit-events`, params: query, signal },
    pageSchema(zApiAuditEvent),
  );
