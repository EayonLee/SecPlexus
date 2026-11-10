"use client";

import Link from "next/link";

import { FileQuestion } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

import { RequireAccess } from "../components/access";
import { ErrorState } from "../components/error-state";
import { settingsPageFor } from "../iam/navigation";
import { AuditPage } from "./audit";
import { RolesPage } from "./roles";
import { UsersPage } from "./users";

// ManagementPage 按路径分发设置区里的管理页面。
// 页面之间用左侧菜单切换，不再有分组标签栏。
export function ManagementPage({ pathname }: { pathname: string }) {
  const page = settingsPageFor(pathname);
  // 路径不存在是 404 语义，不是权限问题：告诉用户地址不对，而不是“无权访问”。
  if (!page) return <UnknownSettingsPath pathname={pathname} />;

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

function UnknownSettingsPath({ pathname }: { pathname: string }) {
  const t = useTranslations("Common");

  return (
    <ErrorState
      icon={FileQuestion}
      title={t("notFoundTitle")}
      description={pathname}
      actions={
        <Button asChild variant="outline" size="sm">
          <Link prefetch={false} replace href="/dashboard">
            {t("backToDashboard")}
          </Link>
        </Button>
      }
    />
  );
}
