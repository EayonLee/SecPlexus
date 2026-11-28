import { NextIntlClientProvider } from "next-intl";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";

import messages from "../../messages/en.json";
import { ListPagination } from "../../src/spx/components/list-controls";

function render({ total, index = 0, hasNext = false }: { total?: number; index?: number; hasNext?: boolean }) {
  return renderToStaticMarkup(
    <NextIntlClientProvider locale="en" messages={messages} timeZone="UTC">
      <ListPagination
        index={index}
        size={10}
        total={total}
        hasNext={hasNext}
        onPage={() => undefined}
        onSize={() => undefined}
      />
    </NextIntlClientProvider>,
  );
}

test("an offset list exposes the four navigation actions even on an empty page", () => {
  const html = render({ total: 0 });
  expect(html).toContain('aria-label="First page"');
  expect(html).toContain('aria-label="Last page"');
  expect(html.match(/<button[^>]+ disabled=""/g)).toHaveLength(4);
});

test("a cursor list exposes only previous and next", () => {
  const html = render({ index: 1, hasNext: true });
  expect(html).not.toContain('aria-label="First page"');
  expect(html).not.toContain('aria-label="Last page"');
  expect(html).toContain('aria-label="Previous page"');
  expect(html).toContain('aria-label="Next page"');
  expect(html).not.toMatch(/<button[^>]+ disabled=""/);
});
