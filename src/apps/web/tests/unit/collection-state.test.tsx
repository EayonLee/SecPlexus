import { NextIntlClientProvider } from "next-intl";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test, vi } from "vitest";

import messages from "../../messages/en.json";
import { CollectionBoundary, CollectionState } from "../../src/spx/components/collection-state";

function render(state: React.ReactNode) {
  return renderToStaticMarkup(
    <NextIntlClientProvider locale="en" messages={messages} timeZone="UTC">
      {state}
    </NextIntlClientProvider>,
  );
}

test("a failed collection exposes recovery and an empty collection has no retry", () => {
  const failure = render(<CollectionState kind="error" onRetry={() => undefined} />);
  expect(failure).toContain('role="alert"');
  expect(failure).toContain("<button");
  const empty = render(<CollectionState kind="empty" />);
  expect(empty).toContain('role="status"');
  expect(empty).not.toContain("<button");
});

test("initial failure replaces the list, while a refresh failure keeps its content during retry", () => {
  const query = { isPending: false, isError: true, isFetching: true, error: new Error(), refetch: vi.fn() };
  const renderList = (hasContent: boolean) =>
    render(
      <CollectionBoundary query={query} hasContent={hasContent}>
        <table aria-label="records">
          <tbody />
        </table>
      </CollectionBoundary>,
    );
  expect(renderList(false)).not.toContain("<table");
  const refreshing = renderList(true);
  expect(refreshing).toContain("<table");
  expect(refreshing).toContain('role="alert"');
  expect(refreshing).toMatch(/<button[^>]*disabled/);
});
