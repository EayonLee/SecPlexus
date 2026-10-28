import { NextIntlClientProvider } from "next-intl";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";

import en from "../../messages/en.json";
import zh from "../../messages/zh-CN.json";
import WorkbenchPage from "../../src/spx/pages/workbench";

test.each([
  { locale: "en" as const, messages: en, title: "Dashboard", leads: "New leads", won: "Projects won" },
  { locale: "zh-CN" as const, messages: zh, title: "安全智驾工作台", leads: "新增销售线索", won: "成交项目" },
])("renders the real dashboard components in $locale", ({ locale, messages, title, leads, won }) => {
  const html = renderToStaticMarkup(
    <NextIntlClientProvider
      locale={locale}
      messages={messages}
      timeZone="UTC"
      now={new Date("2026-10-28T00:00:00Z")}
      onError={(error) => {
        throw error;
      }}
    >
      <WorkbenchPage />
    </NextIntlClientProvider>,
  );
  expect(html).toContain(title);
  expect(html).toContain(leads);
  expect(html).toContain(won);
});
