import { hasLocale } from "next-intl";

export const locales = ["en", "zh-CN"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "zh-CN";
export const localeCookie = "spx-locale";
/** 浏览器时区缓存，由启动脚本写入，供服务端渲染读取。 */
export const timeZoneCookie = "spx-timezone";
/** 用户显式保存的时区偏好，来自账号设置；优先于浏览器时区。 */
export const timeZonePreferenceCookie = "spx-timezone-preference";
export const languageNames: Record<Locale, string> = { en: "English", "zh-CN": "简体中文" };

export function resolveLocale(value: string | undefined): Locale {
  return hasLocale(locales, value) ? value : defaultLocale;
}

export const openGraphLocales: Record<Locale, string> = { en: "en_US", "zh-CN": "zh_CN" };

export function oppositeLocale(locale: string): Locale {
  return locale === "zh-CN" ? "en" : "zh-CN";
}
