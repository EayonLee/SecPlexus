import { Building2, ScrollText, Users } from "lucide-react";

import type { PermissionKey } from "@/auth/permissions.gen";
import { permissions } from "@/auth/permissions.gen";
import type { ApiMe } from "@/spx/api/iam/schemas";

import type messages from "../../../messages/en.json";

export function organizationFromPath(pathname: string, search = ""): string | undefined {
  return pathname.match(/^\/management\/orgs\/([^/]+)/)?.[1] || new URLSearchParams(search).get("org_id") || undefined;
}
export interface ManagementPage {
  id: string;
  title: keyof (typeof messages)["Navigation"];
  group: "people" | "organization" | "activity";
  scope: "organization" | "platform";
  action: PermissionKey;
  path: (orgID?: string) => string;
}
export const managementPages: ManagementPage[] = [
  {
    id: "members",
    title: "members",
    group: "people",
    scope: "organization",
    action: permissions.IAMMembersRead,
    path: (id) => `/management/orgs/${id}/members`,
  },
  {
    id: "roles",
    title: "roles",
    group: "people",
    scope: "organization",
    action: permissions.IAMRolesRead,
    path: (id) => `/management/orgs/${id}/roles`,
  },
  {
    id: "users",
    title: "users",
    group: "people",
    scope: "platform",
    action: permissions.IAMPlatformUsersRead,
    path: () => "/management/platform/users",
  },
  {
    id: "overview",
    title: "organizationSettings",
    group: "organization",
    scope: "organization",
    action: permissions.IAMOrganizationsRead,
    path: (id) => `/management/orgs/${id}/overview`,
  },
  {
    id: "organizations",
    title: "organizations",
    group: "organization",
    scope: "platform",
    action: permissions.IAMPlatformOrganizationsRead,
    path: () => "/management/platform/organizations",
  },
  {
    id: "audit",
    title: "organizationAudit",
    group: "activity",
    scope: "organization",
    action: permissions.IAMAuditRead,
    path: (id) => `/management/orgs/${id}/audit`,
  },
  {
    id: "platform-audit",
    title: "platformAudit",
    group: "activity",
    scope: "platform",
    action: permissions.IAMPlatformAuditRead,
    path: () => "/management/platform/audit",
  },
];
export function visibleManagementPages(identity: Pick<ApiMe, "org_id" | "capabilities" | "platform_capabilities">) {
  return managementPages.filter((page) =>
    page.scope === "platform"
      ? identity.platform_capabilities.includes(page.action)
      : !!identity.org_id && identity.capabilities.includes(page.action),
  );
}

export const managementSections = [
  { id: "people", icon: Users },
  { id: "organization", icon: Building2 },
  { id: "activity", icon: ScrollText },
] as const;

export function visibleManagementSections(identity: Pick<ApiMe, "org_id" | "capabilities" | "platform_capabilities">) {
  const pages = visibleManagementPages(identity);
  return managementSections.flatMap((section) => {
    const first = pages.find((page) => page.group === section.id);
    return first ? [{ ...section, path: first.path }] : [];
  });
}
