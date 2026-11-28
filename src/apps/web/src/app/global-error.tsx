"use client";
import { useEffect, useState } from "react";

import { CircleAlert } from "lucide-react";
import { createTranslator } from "next-intl";

import { messageFallback, resolveLocale } from "@/i18n/config";
import en from "../../messages/en.json";
import zhCN from "../../messages/zh-CN.json";

const catalogs = { en, "zh-CN": zhCN } as const;

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  // 冷启动失败时靠默认设置显示。运行中失败时保留上一次成功渲染的文档语言和主题。
  const [{ locale, colorScheme }] = useState(() => {
    const root = typeof document === "undefined" ? undefined : document.documentElement;
    let colorScheme = "light dark";
    if (root?.dataset.themeMode) colorScheme = root.classList.contains("dark") ? "dark" : "light";
    return {
      locale: resolveLocale(root?.lang),
      colorScheme,
    };
  });
  useEffect(() => {
    console.error(JSON.stringify({ service: "web", event: "ui.global_error", digest: error.digest ?? null }));
  }, [error]);
  const t = createTranslator({ locale, messages: catalogs[locale], getMessageFallback: messageFallback });

  return (
    <html lang={locale} className={colorScheme === "dark" ? "dark" : undefined} style={{ colorScheme }}>
      <body
        style={{
          margin: 0,
          minHeight: "100dvh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "var(--background, Canvas)",
          color: "var(--foreground, CanvasText)",
          fontFamily:
            "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', 'Helvetica Neue', Arial, 'Noto Sans SC', sans-serif",
        }}
      >
        <main style={{ maxWidth: 420, padding: "0 24px", textAlign: "center" }} suppressHydrationWarning>
          <CircleAlert
            aria-hidden="true"
            size={22}
            style={{
              padding: 10,
              marginBottom: 12,
              borderRadius: 10,
              color: "var(--status-warning, CanvasText)",
              background: "var(--muted, ButtonFace)",
            }}
          />
          <h1 style={{ margin: 0, fontSize: "1.5rem", lineHeight: "2rem", letterSpacing: "-0.025em", fontWeight: 600 }}>
            {t("Common.unavailable")}
          </h1>
          <p
            style={{
              marginTop: 8,
              marginBottom: 20,
              fontSize: 14,
              lineHeight: 1.6,
              color: "var(--muted-foreground, CanvasText)",
            }}
          >
            {t("Common.errorDescription")}
          </p>
          <button
            type="button"
            onClick={reset}
            style={{
              appearance: "none",
              border: "1px solid var(--border, ButtonBorder)",
              borderRadius: "var(--radius, 8px)",
              background: "var(--card, ButtonFace)",
              color: "inherit",
              cursor: "pointer",
              font: "inherit",
              padding: "8px 16px",
            }}
          >
            {t("Common.retry")}
          </button>
        </main>
      </body>
    </html>
  );
}
