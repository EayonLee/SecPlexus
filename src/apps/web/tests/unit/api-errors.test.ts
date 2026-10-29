import { afterAll, afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";

import { http } from "../../src/spx/api/client";
import { me } from "../../src/spx/api/iam/auth";
import { useIdentity } from "../../src/spx/iam/session";
import { createServer } from "node:http";

const id = "bfdb6dba-d146-43a0-a9e1-251e0c955442";
let status = 401;
const server = createServer((_request, response) => {
  response.writeHead(status, { "Content-Type": "text/plain", "X-Request-ID": id });
  response.end("secret-upstream-error");
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing owned listener");
  http.defaults.baseURL = `http://127.0.0.1:${address.port}`;
});
beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  useIdentity.setState({ status: "authenticated" });
});
afterEach(() => vi.restoreAllMocks());
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});
it("expires identity on a non-JSON 401 and keeps the safe request ID", async () => {
  status = 401;
  await expect(me()).rejects.toMatchObject({ status: 401, code: "unauthenticated", requestID: id });
  expect(useIdentity.getState().status).toBe("anonymous");
  expect(JSON.stringify(vi.mocked(console.error).mock.calls)).not.toContain("secret-");
});
it("marks a malformed 503 unavailable without logging its body", async () => {
  status = 503;
  await expect(me()).rejects.toMatchObject({ status: 503, code: "service_unavailable" });
  expect(useIdentity.getState().status).toBe("unavailable");
  expect(JSON.stringify(vi.mocked(console.error).mock.calls)).not.toContain("secret-");
});
