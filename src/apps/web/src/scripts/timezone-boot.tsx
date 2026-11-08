/**
 * 把浏览器时区写入 cookie，供服务端渲染读取。
 *
 * 服务端无法得知访问者时区，而 next-intl 的 formatter 时区在请求期确定。
 * 若首屏用 UTC、水合后再改成本地时区，会闪烁并触发水合不一致。
 * 因此用启动脚本在最早期写入 `spx-timezone`，之后的请求服务端即可直接读取。
 *
 * 首次访问时该 cookie 尚不存在，服务端回落 UTC 一次；脚本写入后后续请求即为本地时区。
 * 用户显式偏好（`spx-timezone-preference`）优先级更高，由登录态或设置页维护。
 */
import { timeZoneCookie } from "@/i18n/config";

const code = `
  (function () {
    try {
      var name = ${JSON.stringify(timeZoneCookie)};
      var current = document.cookie.split("; ").find(function (c) {
        return c.startsWith(name + "=");
      });
      var value = Intl.DateTimeFormat().resolvedOptions().timeZone;
      if (!value) { return; }
      if (current && decodeURIComponent(current.split("=")[1]) === value) { return; }
      document.cookie =
        name + "=" + encodeURIComponent(value) +
        "; path=/; max-age=31536000; SameSite=Lax";
    } catch (e) {}
  })();
`;

export function TimeZoneBootScript() {
  /* biome-ignore lint/security/noDangerouslySetInnerHtml: required for pre-hydration boot script */
  return <script dangerouslySetInnerHTML={{ __html: code }} />;
}
