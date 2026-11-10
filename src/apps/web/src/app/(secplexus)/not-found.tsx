"use client";
import Link from "next/link";

import { FileQuestion } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

import { ErrorState } from "@/spx/components/error-state";

// 应用内未找到：保留外壳与导航，用户可以直接换到别的页面。
export default function GroupNotFound() {
  const t = useTranslations("Common");

  return (
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
  );
}
