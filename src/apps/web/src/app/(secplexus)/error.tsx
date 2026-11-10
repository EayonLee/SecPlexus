"use client";
import { useEffect } from "react";

import Link from "next/link";

import { ServerCrash } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

import { ErrorState } from "@/spx/components/error-state";

// 路由级错误边界：页面渲染或数据加载抛错时替换内容区，外壳与导航保留。
export default function RouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const t = useTranslations("Common");

  useEffect(() => {
    // 只记录稳定字段：错误正文可能含业务数据，进浏览器日志即可。
    console.error(JSON.stringify({ service: "web", event: "ui.error_boundary", digest: error.digest ?? null }));
  }, [error]);

  return (
    <ErrorState
      icon={ServerCrash}
      title={t("errorTitle")}
      description={t("errorDescription")}
      actions={
        <>
          <Button size="sm" onClick={reset}>
            {t("retry")}
          </Button>
          <Button asChild variant="outline" size="sm">
            <Link prefetch={false} replace href="/dashboard">
              {t("backToDashboard")}
            </Link>
          </Button>
        </>
      }
    />
  );
}
