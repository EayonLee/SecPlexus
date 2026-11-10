import { ScrollText, ShieldCheck, Users } from "lucide-react";

import type { PermissionKey } from "@/auth/permissions.gen";
import { permissions } from "@/auth/permissions.gen";
import type { ApiMe } from "@/spx/api/iam/schemas";

import type messages from "../../../messages/en.json";

export interface SettingsPage {
  id: string;
  title: keyof (typeof messages)["Navigation"];
  action: PermissionKey;
  path: string;
  icon: typeof ShieldCheck;
}

// settingsPages 是设置区的完整页面列表，按左侧菜单的实际顺序排列。
// 设置主页（个人资料与账号）不在列表里：它总是第一项且始终可见。
export const settingsPages: SettingsPage[] = [
  {
    id: "roles",
    title: "roles",
    action: permissions.IAMRolesRead,
    path: "/settings/roles",
    icon: ShieldCheck,
  },
  {
    id: "users",
    title: "users",
    action: permissions.IAMUsersRead,
    path: "/settings/users",
    icon: Users,
  },
  {
    id: "audit",
    title: "audit",
    action: permissions.IAMAuditRead,
    path: "/settings/audit",
    icon: ScrollText,
  },
];

export function visibleSettingsPages(me: Pick<ApiMe, "permissions">) {
  return settingsPages.filter((page) => me.permissions.includes(page.action));
}

export function settingsPageFor(pathname: string) {
  return settingsPages.find((page) => page.path === pathname);
}
