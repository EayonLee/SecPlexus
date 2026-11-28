import { NextIntlClientProvider } from "next-intl";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";

import en from "../../messages/en.json";
import zh from "../../messages/zh-CN.json";
import { ListSelect } from "../../src/spx/components/list-controls";

function render(locale: "en" | "zh-CN", messages: typeof en | { Common: object }, label: string, all: string) {
  return renderToStaticMarkup(
    <NextIntlClientProvider
      locale={locale}
      messages={messages}
      timeZone="UTC"
      onError={(error) => {
        throw error;
      }}
    >
      <ListSelect label={label} value="all" options={[{ value: "all", label: all }]} onChange={() => undefined} />
    </NextIntlClientProvider>,
  );
}

test.each([
  { locale: "en" as const, messages: en, label: "Protocol", all: "All protocols" },
  { locale: "zh-CN" as const, messages: zh, label: "协议", all: "所有协议" },
])("keeps the filter name and current value available in $locale", ({ locale, messages, label, all }) => {
  const markup = render(locale, messages, label, all);
  expect(markup).toContain(messages.Common.filterLabel.replace("{label}", label));
  expect(markup).toContain(`>${messages.Common.allOptions}</span>`);

  const partial = render(locale, { Common: {} }, label, all);
  expect(partial).toContain(`>${label}</span>`);
  expect(partial).toContain(`>${all}</span>`);
});
