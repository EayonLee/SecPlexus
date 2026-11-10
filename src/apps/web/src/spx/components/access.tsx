"use client";
import { type ReactNode, useEffect } from "react";

import { useRouter } from "next/navigation";

import { useTranslations } from "next-intl";

import { CloudOff } from "lucide-react";

import type { PermissionKey } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";

import { initializeSession, refreshMe, useIdentity } from "../iam/session";
import { ErrorState } from "./error-state";
import { LanguageSwitcher } from "./language-switcher";

export function useCan(action: PermissionKey) {
  const auth = useIdentity();
  return auth.status === "authenticated" && auth.permissions.includes(action);
}
export function Can({ action, children }: { action: PermissionKey; children: ReactNode }) {
  return useCan(action) ? children : null;
}
export function AccessDenied() {
  const t = useTranslations("Common");

  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>{t("accessDenied")}</EmptyTitle>
        <EmptyDescription>{t("accessDeniedDescription")}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}
export function RequireAccess({ action, children }: { action: PermissionKey; children: ReactNode }) {
  return useCan(action) ? children : <AccessDenied />;
}
export function AuthBoundary({ children }: { children: ReactNode }) {
  const t = useTranslations("Common");

  const auth = useIdentity();
  const router = useRouter();
  useEffect(() => {
    void initializeSession();
  }, []);
  useEffect(() => {
    if (auth.status === "anonymous") router.replace("/login");
  }, [auth.status, router]);
  if (auth.status === "unknown" || auth.status === "anonymous")
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <Spinner />
      </div>
    );
  if (auth.status === "unavailable")
    return (
      <div className="relative">
        <div className="absolute top-4 right-4">
          <LanguageSwitcher />
        </div>
        <ErrorState
          icon={CloudOff}
          title={t("unavailable")}
          description={t("paused")}
          actions={
            <Button size="sm" variant="outline" onClick={() => void refreshMe()}>
              {t("reconnect")}
            </Button>
          }
        />
      </div>
    );
  return children;
}
