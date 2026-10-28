import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { api, useIdentity } from "../../src/spx/iam/session";

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  useIdentity.setState({ status: "authenticated" });
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("expires identity on a non-JSON 401 and keeps the safe request ID", async () => {
  const id = "bfdb6dba-d146-43a0-a9e1-251e0c955442";
  vi.mocked(fetch).mockResolvedValue(
    new Response("secret-response-body", { status: 401, headers: { "X-Request-ID": id } }),
  );
  await expect(api("/iam/me")).rejects.toMatchObject({ status: 401, code: "unauthenticated", requestID: id });
  expect(useIdentity.getState().status).toBe("anonymous");
  expect(JSON.stringify(vi.mocked(console.error).mock.calls)).not.toContain("secret-");
});

it("marks a malformed 503 unavailable without logging its body", async () => {
  vi.mocked(fetch).mockResolvedValue(new Response("secret-upstream-error", { status: 503 }));
  await expect(api("/iam/me")).rejects.toMatchObject({ status: 503, code: "service_unavailable" });
  expect(useIdentity.getState().status).toBe("unavailable");
  expect(JSON.stringify(vi.mocked(console.error).mock.calls)).not.toContain("secret-");
});
