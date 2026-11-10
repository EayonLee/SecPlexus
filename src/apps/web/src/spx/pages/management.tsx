"use client";

import { AccessDenied, RequireAccess } from "../components/access";
import { settingsPageFor } from "../iam/navigation";
import { AuditPage } from "./audit";
import { RolesPage } from "./roles";
import { UsersPage } from "./users";

// ManagementPage 按路径分发设置区里的管理页面。
// 页面之间用左侧菜单切换，不再有分组标签栏。
export function ManagementPage({ pathname }: { pathname: string }) {
  const page = settingsPageFor(pathname);
  if (!page) return <AccessDenied />;

  return (
    <RequireAccess action={page.action}>
      <div className="@container/main flex flex-col gap-4 md:gap-6">
        {page.id === "roles" && <RolesPage />}
        {page.id === "users" && <UsersPage />}
        {page.id === "audit" && <AuditPage />}
      </div>
    </RequireAccess>
  );
}
