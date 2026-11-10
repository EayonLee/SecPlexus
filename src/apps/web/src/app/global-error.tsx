"use client";
import { useEffect, useState } from "react";

import { createTranslator } from "next-intl";

import { localeCookie, resolveLocale } from "@/i18n/config";
import en from "../../messages/en.json";
import zhCN from "../../messages/zh-CN.json";

const catalogs = { en, "zh-CN": zhCN } as const;

// 根布局自身失败时的最后一道界面：没有 Provider、也没有全局样式，
// 因此文案自带、样式内联，保证任何情况下都能显示。
function readLocale(): keyof typeof catalogs {
  if (typeof document === "undefined") return "zh-CN";
  const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${localeCookie}=([^;]+)`));
  return resolveLocale(match?.[1]);
}

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const [locale, setLocale] = useState<keyof typeof catalogs>("zh-CN");
  useEffect(() => {
    setLocale(readLocale());
    console.error(JSON.stringify({ service: "web", event: "ui.global_error", digest: error.digest ?? null }));
  }, [error]);
  const t = createTranslator({ locale, messages: catalogs[locale] });

  return (
    <html lang={locale}>
      <body
        style={{
          margin: 0,
          minHeight: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#0b0b0c",
          color: "#f5f5f5",
          fontFamily:
            "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', 'Helvetica Neue', Arial, 'Noto Sans SC', sans-serif",
        }}
      >
        <main style={{ maxWidth: 420, padding: "0 24px", textAlign: "center" }} suppressHydrationWarning>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>{t("Common.unavailable")}</h1>
          <p style={{ marginTop: 8, marginBottom: 20, fontSize: 14, lineHeight: 1.6, color: "#a1a1aa" }}>
            {t("Common.errorDescription")}
          </p>
          <button
            type="button"
            onClick={reset}
            style={{
              appearance: "none",
              border: "1px solid #3f3f46",
              borderRadius: 8,
              background: "transparent",
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
