"use client";
import Link from "next/link";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { LanguageSwitcher } from "@/spx/components/language-switcher";

export default function NotFound() {
  const t = useTranslations("Common");

  return (
    <div className="relative flex h-dvh flex-col items-center justify-center space-y-2 text-center">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher />
      </div>
      <h1 className="text-2xl font-semibold">{t("notFoundTitle")}</h1>
      <p className="text-muted-foreground">{t("notFoundDescription")}</p>
      <Link prefetch={false} replace href="/dashboard">
        <Button variant="outline">{t("backToDashboard")}</Button>
      </Link>
    </div>
  );
}
