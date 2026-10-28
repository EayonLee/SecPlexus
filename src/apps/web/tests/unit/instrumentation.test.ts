import { afterEach, expect, it, vi } from "vitest";

import { onRequestError } from "../../src/instrumentation";

afterEach(() => vi.restoreAllMocks());

it("records a safe Next server error with route and digest, excluding request and error values", () => {
  const output = vi.spyOn(console, "error").mockImplementation(() => undefined);
  const error = Object.assign(new TypeError("secret-password"), { digest: "123456" });
  onRequestError(
    error,
    { path: "/secret-path?token=secret-query", method: "POST", headers: { cookie: "secret-cookie" } },
    { routerKind: "App Router", routePath: "/items/[id]", routeType: "action", revalidateReason: undefined },
  );
  const text = output.mock.calls[0][0] as string;
  const record = JSON.parse(text);
  expect(record).toMatchObject({
    service: "web",
    event: "web.request_failed",
    route: "/items/[id]",
    error_kind: "TypeError",
    digest: "123456",
  });
  expect(record.request_id).toBeTruthy();
  expect(text).not.toContain("secret-");
});
