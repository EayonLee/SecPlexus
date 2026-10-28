import { cookies } from "next/headers";

import { getRequestConfig } from "next-intl/server";

import { localeCookie, resolveLocale, timeZone } from "./config";

const catalogs = {
  en: () => import("../../messages/en.json"),
  "zh-CN": () => import("../../messages/zh-CN.json"),
};

export default getRequestConfig(async () => {
  const locale = resolveLocale((await cookies()).get(localeCookie)?.value);
  return { locale, messages: (await catalogs[locale]()).default, timeZone, now: new Date() };
});
