import { hasLocale } from "next-intl";

export const locales = ["en", "zh-CN"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "en";
export const localeCookie = "spx-locale";
export const timeZone = "UTC";
export const languageNames: Record<Locale, string> = { en: "English", "zh-CN": "简体中文" };

export function resolveLocale(value: string | undefined): Locale {
  return hasLocale(locales, value) ? value : defaultLocale;
}

export const openGraphLocales: Record<Locale, string> = { en: "en_US", "zh-CN": "zh_CN" };

export function oppositeLocale(locale: string): Locale {
  return locale === "zh-CN" ? "en" : "zh-CN";
}
