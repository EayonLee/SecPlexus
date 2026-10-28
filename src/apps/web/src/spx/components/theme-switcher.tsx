"use client";
import { Moon, Sun } from "lucide-react";
import { useTranslations } from "next-intl";
import { useShallow } from "zustand/react/shallow";

import { Button } from "@/components/ui/button";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

export function ThemeSwitcher() {
  const t = useTranslations("Appearance");

  const { resolvedThemeMode, setPreference } = usePreferencesStore(
    useShallow((state) => ({
      resolvedThemeMode: state.resolvedThemeMode,
      setPreference: state.setPreference,
    })),
  );

  const nextTheme = resolvedThemeMode === "dark" ? "light" : "dark";

  return (
    <Button
      size="icon"
      onClick={() => setPreference("theme_mode", nextTheme)}
      aria-label={nextTheme === "dark" ? t("switchDark") : t("switchLight")}
    >
      <Sun className="hidden dark:block" />
      <Moon className="block dark:hidden" />
    </Button>
  );
}
