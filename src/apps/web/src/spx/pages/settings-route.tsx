"use client";

import { usePathname } from "next/navigation";

import { useIdentity } from "../iam/session";
import { ManagementPage } from "./management";
import { SettingsPage } from "./settings";

// SettingsRoute 把设置区收在一个入口下：
// `/settings` 是个人与账号设置，`/settings/**` 是角色、组织、审计等管理页面。
export function SettingsRoute() {
  const pathname = usePathname();
  const auth = useIdentity();

  // 只有精确命中 /settings 才是设置主页；其余子路径交给管理页面分发。
  if (pathname === "/settings") return <SettingsPage />;

  return <ManagementPage orgID={auth.org_id} pathname={pathname} />;
}
