"use client";

import { useTransition } from "react";

import { Languages } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { setLocale } from "@/i18n/actions";
import { languageNames, oppositeLocale } from "@/i18n/config";

import { useAction } from "./data";

export function LanguageSwitcher() {
  const perform = useAction();
  const locale = useLocale();
  const t = useTranslations("Common");
  const [pending, startTransition] = useTransition();
  const nextLocale = oppositeLocale(locale);
  return (
    <Button
      variant="ghost"
      size="icon"
      disabled={pending}
      aria-label={t("language")}
      title={languageNames[nextLocale]}
      onClick={() =>
        startTransition(async () => {
          await perform(() => setLocale(nextLocale));
        })
      }
    >
      <Languages />
    </Button>
  );
}
