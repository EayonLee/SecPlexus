"use client";

import { AccessDenied, RequireAccess } from "../components/access";
import { settingsPageFor } from "../iam/navigation";
import { AuditPage } from "./audit";
import { OrganizationsPage } from "./organizations";
import { RolesPage } from "./roles";
import { UsersPage } from "./users";

// ManagementPage 按路径分发设置区里的管理页面。
// 页面之间用左侧菜单切换，不再有分组标签栏。
export function ManagementPage({ orgID, pathname }: { orgID?: string; pathname: string }) {
  const page = settingsPageFor(pathname, orgID);
  if (!page) return <AccessDenied />;

  return (
    <RequireAccess action={page.action}>
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
        {page.id === "organizations" && <OrganizationsPage />}
        {page.id === "roles" && <RolesPage key={orgID} />}
        {page.id === "users" && <UsersPage key={orgID} />}
        {page.id === "audit" && <AuditPage />}
      </div>
    </RequireAccess>
  );
}
