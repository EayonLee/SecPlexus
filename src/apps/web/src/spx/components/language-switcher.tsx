"use client";

import { useTransition } from "react";

import { Languages } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { setLocale } from "@/i18n/actions";
import { languageNames, locales } from "@/i18n/config";

import { useAction } from "./data";

export function LanguageSwitcher() {
  const perform = useAction();
  const locale = useLocale();
  const t = useTranslations("Common");
  const [pending, startTransition] = useTransition();
  function changeLocale(value: string) {
    const nextLocale = locales.find((option) => option === value);
    if (!nextLocale || nextLocale === locale) return;
    startTransition(async () => {
      await perform(async () => {
        await setLocale(nextLocale);
      });
    });
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" disabled={pending} aria-label={t("language")} title={t("language")}>
          <Languages />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup value={locale} onValueChange={changeLocale}>
          {locales.map((option) => (
            <DropdownMenuRadioItem key={option} value={option}>
              {languageNames[option]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
