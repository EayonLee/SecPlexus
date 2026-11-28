import { createFormatter, createTranslator } from "next-intl";
import { describe, expect, test } from "vitest";

import en from "../../messages/en.json";
import zh from "../../messages/zh-CN.json";
import { messageFallback, oppositeLocale, resolveLocale } from "../../src/i18n/config";

function keys(value: object, prefix = ""): string[] {
  return Object.entries(value).flatMap(([key, entry]) =>
    typeof entry === "string" ? [`${prefix}${key}`] : keys(entry, `${prefix}${key}.`),
  );
}

function messages(value: object): string[] {
  return Object.values(value).flatMap((entry) => (typeof entry === "string" ? [entry] : messages(entry)));
}

describe("platform localization", () => {
  test("supports explicit language preferences and falls back safely", () => {
    expect(resolveLocale("zh-CN")).toBe("zh-CN");
    expect(resolveLocale("en")).toBe("en");
    expect(resolveLocale("unsupported")).toBe("zh-CN");
    expect(resolveLocale(undefined)).toBe("zh-CN");
  });

  test("provides the same message keys in both languages", () => {
    expect(keys(zh).sort()).toEqual(keys(en).sort());
  });

  test("keeps a missing key out of the UI and preserves its diagnostic", () => {
    const errors: string[] = [];
    const catalog = structuredClone(zh);
    Reflect.deleteProperty(catalog.Common, "allOptions");
    const translate = createTranslator({
      locale: "zh-CN",
      messages: catalog,
      getMessageFallback: messageFallback,
      onError: (error) => errors.push(error.code),
    });
    expect(translate("Common.allOptions")).toBe("—");
    expect(errors).toEqual(["MISSING_MESSAGE"]);
  });

  test("uses the approved dashboard names and ICU pagination", () => {
    const english = createTranslator({ locale: "en", messages: en });
    const chinese = createTranslator({ locale: "zh-CN", messages: zh });
    expect(english("Dashboard.title")).toBe("Security Operations");
    expect(chinese("Dashboard.title")).toBe("安全运营工作台");
    expect(english("Common.pagination", { total: 1, page: 1 })).toBe("1 record · Page 1");
    expect(english("Common.pagination", { total: 2, page: 1 })).toBe("2 records · Page 1");
    expect(chinese("Common.pagination", { total: 2, page: 1 })).toBe("共 2 条 · 第 1 页");
  });

  test("all catalog messages have valid ICU syntax and declared interpolation values", () => {
    for (const [locale, catalog] of [
      ["en", en],
      ["zh-CN", zh],
    ] as const) {
      for (const value of messages(catalog)) {
        const translate = createTranslator({
          locale,
          messages: { value },
          onError: (error) => {
            throw error;
          },
        });
        expect(
          translate("value", {
            names: "Edge",
            fields: "Source IP",
            carried: 1,
            version: 2,
            passed: 1,
            field: "message",
            source: "host",
            target: "normalized_host",
            count: 2,
            label: "TCP",
            total: 2,
            limit: 120,
            first: 1,
            last: 2,
            current: 1,
            title: "Example",
            number: 1,
            key: "example",
            level: "2",
            code: "invalid_value",
            inputs: "Example input",
            matched: 1,
            selected: 1,
            page: 1,
            added: "read",
            removed: "write",
            name: "Example",
            month: "June",
            range: "1–5",
            type: "JSON",
          }),
        ).not.toBe("");
      }
    }
  });

  test("formats money without changing currency and keeps timestamp seconds", () => {
    const english = createFormatter({ locale: "en", timeZone: "UTC" });
    const chinese = createFormatter({ locale: "zh-CN", timeZone: "UTC" });
    const date = new Date("2026-10-28T10:18:27Z");
    expect(english.number(1234.5, { style: "currency", currency: "USD" })).toBe("$1,234.50");
    expect(chinese.number(1234.5, { style: "currency", currency: "USD" })).toContain("1,234.50");
    for (const formatter of [english, chinese]) {
      expect(formatter.dateTime(date, { dateStyle: "medium", timeStyle: "medium" })).toContain("18:27");
    }
  });
});

test("language toggle alternates only between the supported English and Chinese locales", () => {
  expect(oppositeLocale("en")).toBe("zh-CN");
  expect(oppositeLocale("zh-CN")).toBe("en");
  expect(oppositeLocale(oppositeLocale("en"))).toBe("en");
});
