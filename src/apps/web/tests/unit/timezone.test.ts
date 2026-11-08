import { describe, expect, test } from "vitest";

import { timeZonePreferenceCookie } from "../../src/i18n/config";
import { isSupportedTimeZone, resolveTimeZone } from "../../src/i18n/timezone";
import { timeZonePreferenceCookieValue } from "../../src/i18n/timezone-cookie";

describe("display time zone resolution", () => {
  test("prefers a concrete user preference over everything else", () => {
    expect(resolveTimeZone({ userTimeZone: "Asia/Shanghai", browserTimeZone: "Europe/Berlin" })).toBe("Asia/Shanghai");
  });

  test("treats browser as a semantic value resolved by the runtime", () => {
    expect(resolveTimeZone({ userTimeZone: "browser", browserTimeZone: "Europe/Berlin" })).toBe("Europe/Berlin");
  });

  test("falls back to the browser zone when the user has no preference", () => {
    expect(resolveTimeZone({ userTimeZone: null, browserTimeZone: "Europe/Berlin" })).toBe("Europe/Berlin");
    expect(resolveTimeZone({ userTimeZone: "", browserTimeZone: "Europe/Berlin" })).toBe("Europe/Berlin");
  });

  test("treats utc as an explicit choice, not a fallback", () => {
    expect(resolveTimeZone({ userTimeZone: "utc", browserTimeZone: "Europe/Berlin" })).toBe("UTC");
  });

  test("falls back to UTC when nothing usable is available", () => {
    expect(resolveTimeZone({ userTimeZone: null, browserTimeZone: null })).toBe("UTC");
    expect(resolveTimeZone({ userTimeZone: null, browserTimeZone: "" })).toBe("UTC");
  });

  test("ignores an unsupported user value instead of leaking it to the formatter", () => {
    expect(resolveTimeZone({ userTimeZone: "Not/AZone", browserTimeZone: "Europe/Berlin" })).toBe("Europe/Berlin");
  });

  test("validates time zones the way the server does", () => {
    expect(isSupportedTimeZone("Asia/Shanghai")).toBe(true);
    expect(isSupportedTimeZone("utc")).toBe(true);
    expect(isSupportedTimeZone("browser")).toBe(true);
    expect(isSupportedTimeZone("")).toBe(true);
    expect(isSupportedTimeZone("Not/AZone")).toBe(false);
    expect(isSupportedTimeZone(undefined)).toBe(false);
  });
});

describe("time zone preference cookie", () => {
  test("writes a durable cookie for a saved preference", () => {
    const cookie = timeZonePreferenceCookieValue("Asia/Shanghai");

    expect(cookie).toContain(`${timeZonePreferenceCookie}=`);
    expect(cookie).toContain("max-age=31536000");
    expect(cookie).toContain("path=/");
    expect(cookie).toContain("SameSite=Lax");
  });

  test("expires the cookie when the preference is cleared", () => {
    for (const cleared of ["", null, undefined]) {
      const cookie = timeZonePreferenceCookieValue(cleared);

      expect(cookie).toContain("max-age=0");
      expect(cookie).not.toContain("Asia");
    }
  });

  test("encodes the slash so the value survives a cookie round trip", () => {
    expect(timeZonePreferenceCookieValue("America/Los_Angeles")).toContain("America%2FLos_Angeles");
  });

  test("encodes semantic values without altering them", () => {
    expect(timeZonePreferenceCookieValue("utc")).toContain(`${timeZonePreferenceCookie}=utc;`);
    expect(timeZonePreferenceCookieValue("browser")).toContain(`${timeZonePreferenceCookie}=browser;`);
  });
});
