import { cookies } from "next/headers";

import { getRequestConfig } from "next-intl/server";

import { apiOrigins } from "@/spx/server/config";

import { localeCookie, resolveLocale, timeZoneCookie, timeZonePreferenceCookie } from "./config";
import { resolveTimeZone } from "./timezone";

const catalogs = {
  en: () => import("../../messages/en.json"),
  "zh-CN": () => import("../../messages/zh-CN.json"),
};

/**
 * 从 IAM 读取本人已保存的时区偏好。
 *
 * cookie 只是渲染期镜像，换设备或清 cookie 后它就没了；此时必须回源数据库，
 * 否则首屏会用浏览器时区渲染一次，随后被客户端纠正，出现错误时区闪烁。
 */
async function savedTimeZone(): Promise<string | null> {
  try {
    const cookieHeader = (await cookies()).toString();
    const response = await fetch(`${apiOrigins.iam}/api/v1/iam/me`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
      signal: AbortSignal.timeout(2000),
    });
    if (!response.ok) return null;

    const body = (await response.json()) as { data?: { timezone?: string | null } };
    const value = body.data?.timezone;
    return typeof value === "string" && value !== "" ? value : null;
  } catch {
    return null;
  }
}

export default getRequestConfig(async () => {
  const store = await cookies();
  const locale = resolveLocale(store.get(localeCookie)?.value);
  const cached = store.get(timeZonePreferenceCookie)?.value;
  const userTimeZone = cached === undefined ? await savedTimeZone() : cached;
  const timeZone = resolveTimeZone({
    userTimeZone,
    browserTimeZone: store.get(timeZoneCookie)?.value,
  });
  return { locale, messages: (await catalogs[locale]()).default, timeZone, now: new Date() };
});
