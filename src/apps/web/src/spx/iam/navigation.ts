import { Building2, ScrollText, ShieldCheck, Users } from "lucide-react";

import type { PermissionKey } from "@/auth/permissions.gen";
import { permissions } from "@/auth/permissions.gen";
import type { ApiMe } from "@/spx/api/iam/schemas";

import type messages from "../../../messages/en.json";

export function organizationFromPath(pathname: string, search = ""): string | undefined {
  return pathname.match(/^\/settings\/orgs\/([^/]+)/)?.[1] || new URLSearchParams(search).get("org_id") || undefined;
}

export interface SettingsPage {
  id: string;
  title: keyof (typeof messages)["Navigation"];
  scope: "organization" | "platform";
  action: PermissionKey;
  path: (orgID?: string) => string;
  icon: typeof Building2;
}

// settingsPages 是设置区的完整页面列表，按左侧菜单的实际顺序排列。
// 设置主页（个人资料与账号）不在列表里：它总是第一项且始终可见。
export const settingsPages: SettingsPage[] = [
  {
    id: "roles",
    title: "roles",
    scope: "organization",
    action: permissions.IAMRolesRead,
    path: (id) => `/settings/orgs/${id}/roles`,
    icon: ShieldCheck,
  },
  {
    id: "organizations",
    title: "organizations",
    scope: "platform",
    action: permissions.IAMPlatformOrganizationsRead,
    path: () => "/settings/platform/organizations",
    icon: Building2,
  },
  {
    id: "users",
    title: "users",
    scope: "organization",
    action: permissions.IAMUsersRead,
    path: (id) => `/settings/orgs/${id}/users`,
    icon: Users,
  },
  {
    id: "audit",
    title: "audit",
    scope: "organization",
    action: permissions.IAMAuditRead,
    path: (id) => `/settings/orgs/${id}/audit`,
    icon: ScrollText,
  },
];

export function visibleSettingsPages(identity: Pick<ApiMe, "org_id" | "capabilities" | "platform_capabilities">) {
  return settingsPages.filter((page) =>
    page.scope === "platform"
      ? identity.platform_capabilities.includes(page.action)
      : !!identity.org_id && identity.capabilities.includes(page.action),
  );
}

export function settingsPageFor(pathname: string, orgID?: string) {
  return settingsPages.find((page) => page.path(orgID) === pathname);
}
