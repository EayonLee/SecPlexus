"use client";
import Link from "next/link";

import { FileQuestion } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

import { ErrorState } from "@/spx/components/error-state";
import { LanguageSwitcher } from "@/spx/components/language-switcher";

export default function NotFound() {
  const t = useTranslations("Common");

  return (
    <div className="relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher />
      </div>
      <ErrorState
        icon={FileQuestion}
        title={t("notFoundTitle")}
        description={t("notFoundDescription")}
        actions={
          <Button asChild variant="outline" size="sm">
            <Link prefetch={false} replace href="/dashboard">
              {t("backToDashboard")}
            </Link>
          </Button>
        }
      />
    </div>
  );
}
