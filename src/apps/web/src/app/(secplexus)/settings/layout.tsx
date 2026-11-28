"use client";

import type { ReactNode } from "react";

import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { profilePath, settingsPageFor } from "@/spx/iam/navigation";

export default function SettingsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const t = useTranslations("Navigation");
  const current = pathname === profilePath ? "profile" : settingsPageFor(pathname)?.title;

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {current && (
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <span>{t("settings")}</span>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t(current)}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      )}
      {children}
    </div>
  );
}
