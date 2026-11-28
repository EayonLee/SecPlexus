"use client";
import { type ReactNode, useEffect } from "react";

import { useRouter } from "next/navigation";

import { CloudOff, ShieldX } from "lucide-react";
import { useTranslations } from "next-intl";

import type { PermissionKey } from "@/auth/permissions.gen";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

import { initializeSession, refreshMe, useIdentity } from "../iam/session";
import { StatePanel } from "./collection-state";
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
    <StatePanel
      pageHeading
      kind="error"
      icon={ShieldX}
      title={t("accessDenied")}
      description={t("accessDeniedDescription")}
    />
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
    if (auth.status === "anonymous") {
      const path = window.location.pathname;
      router.replace(
        path === "/ingest" || path.startsWith("/ingest/")
          ? `/login?next=${encodeURIComponent(path + window.location.search)}`
          : "/login",
      );
    }
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
          viewport
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
