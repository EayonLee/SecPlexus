import { afterEach, expect, it, vi } from "vitest";

import { GET } from "../../src/app/api/v1/[module]/[...path]/route";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("returns one safe 503 Result with correlated headers and controlled dependency diagnostics", async () => {
  const logs = vi.spyOn(console, "error").mockImplementation(() => undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(
      new TypeError("secret-upstream-url", {
        cause: Object.assign(new Error("secret-connection"), { code: "ECONNREFUSED" }),
      }),
    ),
  );
  const response = await GET(
    new Request("http://localhost/api/v1/iam/me?token=secret-query", {
      headers: { Cookie: "secret-cookie" },
    }),
  );
  const result = await response.json();
  expect(response.status).toBe(503);
  expect(result).toMatchObject({ code: "service_unavailable", data: null });
  expect(response.headers.get("X-Request-ID")).toBe(result.request_id);
  expect(JSON.parse(logs.mock.calls[0][0])).toMatchObject({
    event: "web.proxy_failed",
    request_id: result.request_id,
    reason: "ECONNREFUSED",
    module: "iam",
  });
  expect(JSON.stringify(logs.mock.calls)).not.toContain("secret-");
  expect(JSON.stringify(result)).not.toContain("secret-");
});
