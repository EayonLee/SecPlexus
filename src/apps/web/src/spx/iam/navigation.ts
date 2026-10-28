import type { PermissionKey } from "@/auth/permissions.gen";
import { permissions } from "@/auth/permissions.gen";
import type { ApiMe } from "@/spx/api/iam/types.gen";

import type messages from "../../../messages/en.json";

export function organizationFromPath(pathname: string, search = ""): string | undefined {
  return pathname.match(/^\/management\/orgs\/([^/]+)/)?.[1] || new URLSearchParams(search).get("org_id") || undefined;
}
export interface ManagementPage {
  id: string;
  title: keyof (typeof messages)["Navigation"];
  group: "organizationAccess" | "platformManagement" | "systemSettings";
  scope: "organization" | "platform";
  action: PermissionKey;
  path: (orgID?: string) => string;
}
export const managementPages: ManagementPage[] = [
  {
    id: "overview",
    title: "organizationSettings",
    group: "organizationAccess",
    scope: "organization",
    action: permissions.IAMOrganizationsRead,
    path: (id) => `/management/orgs/${id}/overview`,
  },
  {
    id: "members",
    title: "members",
    group: "organizationAccess",
    scope: "organization",
    action: permissions.IAMMembersRead,
    path: (id) => `/management/orgs/${id}/members`,
  },
  {
    id: "roles",
    title: "roles",
    group: "organizationAccess",
    scope: "organization",
    action: permissions.IAMRolesRead,
    path: (id) => `/management/orgs/${id}/roles`,
  },
  {
    id: "audit",
    title: "organizationAudit",
    group: "organizationAccess",
    scope: "organization",
    action: permissions.IAMAuditRead,
    path: (id) => `/management/orgs/${id}/audit`,
  },
  {
    id: "users",
    title: "users",
    group: "platformManagement",
    scope: "platform",
    action: permissions.IAMPlatformUsersRead,
    path: () => "/management/platform/users",
  },
  {
    id: "organizations",
    title: "organizations",
    group: "platformManagement",
    scope: "platform",
    action: permissions.IAMPlatformOrganizationsRead,
    path: () => "/management/platform/organizations",
  },
  {
    id: "platform-audit",
    title: "platformAudit",
    group: "platformManagement",
    scope: "platform",
    action: permissions.IAMPlatformAuditRead,
    path: () => "/management/platform/audit",
  },
  {
    id: "security",
    title: "authenticationSecurity",
    group: "systemSettings",
    scope: "platform",
    action: permissions.IAMPlatformSecurityRead,
    path: () => "/management/system/security",
  },
];
export function visibleManagementPages(identity: Pick<ApiMe, "org_id" | "capabilities" | "platform_capabilities">) {
  return managementPages.filter((page) =>
    page.scope === "platform"
      ? identity.platform_capabilities.includes(page.action)
      : !!identity.org_id && identity.capabilities.includes(page.action),
  );
}
