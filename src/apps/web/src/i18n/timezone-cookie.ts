import { timeZonePreferenceCookie } from "./config";

/**
 * 生成时区偏好 cookie 的序列化值，null 表示应删除。
 *
 * 偏好真源在数据库；cookie 只是服务端渲染期的镜像，必须随登录会话同步，
 * 否则换账号会继承上一人的时区，换设备又读不到自己已保存的偏好。
 */
export function timeZonePreferenceCookieValue(value: string | null | undefined): string {
  if (typeof value !== "string" || value === "") {
    return `${timeZonePreferenceCookie}=; path=/; max-age=0; SameSite=Lax`;
  }

  return `${timeZonePreferenceCookie}=${encodeURIComponent(value)}; path=/; max-age=31536000; SameSite=Lax`;
}
