import { NextIntlClientProvider } from "next-intl";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";

import en from "../../messages/en.json";
import zh from "../../messages/zh-CN.json";
import { ManagementPage } from "../../src/spx/pages/management";
import GroupNotFound from "../../src/app/(secplexus)/not-found";
import RouteError from "../../src/app/(secplexus)/error";

function render(node: React.ReactNode, locale: "en" | "zh-CN", messages: typeof en) {
  return renderToStaticMarkup(
    <NextIntlClientProvider locale={locale} messages={messages} timeZone="UTC" onError={(error) => {
      throw error;
    }}>
      {node}
    </NextIntlClientProvider>,
  );
}

test.each([
  { locale: "zh-CN" as const, messages: zh },
  { locale: "en" as const, messages: en },
])("error surfaces share one layout in $locale", ({ locale, messages }) => {
  const { notFoundTitle: notFound, errorTitle: error, retry, backToDashboard: back } = messages.Common;
  const notFoundHtml = render(<GroupNotFound />, locale, messages as typeof en);
  expect(notFoundHtml).toContain(notFound);
  expect(notFoundHtml).toContain(back);

  const errorHtml = render(
    <RouteError
      error={Object.assign(new Error("boom"), { digest: "digest-1" })}
      reset={() => {
        // 静态渲染不会触发重试，这里只需要一个可调用的回调。
      }}
    />,
    locale,
    messages as typeof en,
  );
  expect(errorHtml).toContain(error);
  expect(errorHtml).toContain(retry);
});

// 不存在的设置子路径是“未找到”，不是“无权访问”。
test("unknown settings path renders not-found, not access-denied", () => {
  const html = render(<ManagementPage pathname="/settings/does-not-exist" />, "zh-CN", zh as typeof en);
  expect(html).toContain("页面未找到");
  expect(html).toContain("/settings/does-not-exist");
  expect(html).not.toContain("无权访问");
});
