import { Database, FileJson, ScrollText, ShieldCheck, Users } from "lucide-react";

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

export const profilePath = "/settings/profile";

// settingsPages 列出需要 IAM 读取权限的设置页。个人资料始终显示在设置导航首项。
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

export const ingestPages: SettingsPage[] = [
  {
    id: "ingest",
    title: "inputs",
    action: permissions.IngestInputRead,
    path: "/ingest/inputs",
    icon: Database,
  },
  {
    id: "parsing-rules",
    title: "mappings",
    action: permissions.IngestParsingRuleRead,
    path: "/ingest/parsing-rules",
    icon: FileJson,
  },
];

export function visibleIngestPages(me: Pick<ApiMe, "permissions">) {
  return ingestPages.filter((page) => me.permissions.includes(page.action));
}
