import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { createServer } from "node:http";

let forwarded: string | undefined;
const server = createServer((request, response) => {
  forwarded = request.headers["x-forwarded-for"] as string | undefined;
  if (request.url?.includes("rejected-login")) {
    // 模拟上游在读完请求体前返回 401；代理必须透传状态而不是改成 503。
    response.writeHead(401, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ code: "iam.invalid_credentials", message: "invalid", data: null }));
    return;
  }
  response.writeHead(405, { Allow: "GET", "Retry-After": "3", "Set-Cookie": ["one=1; HttpOnly", "two=2; HttpOnly"] });
  response.end();
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing test listener");
  vi.stubEnv("SPX_IAM_HOST", "127.0.0.1");
  vi.stubEnv("SPX_IAM_PORT", String(address.port));
  vi.stubEnv("SPX_CONSOLE_HOST", "127.0.0.1");
  vi.stubEnv("SPX_CONSOLE_PORT", "18080");
  vi.stubEnv("SPX_WEB_PORT", "15173");
});
afterAll(async () => {
  vi.unstubAllEnvs();
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});
test("passes through an upstream rejection with a request body", async () => {
  const { POST } = await import("../../src/app/api/v1/[module]/[...path]/route");
  const response = await POST(
    new Request("http://localhost/api/v1/iam/rejected-login", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ username: "admin", password: "wrong" }),
    }),
  );
  expect(response.status).toBe(401);
  expect(await response.json()).toMatchObject({ code: "iam.invalid_credentials" });
});

test("same-origin forwarding preserves method negotiation and repeated cookies", async () => {
  const { POST } = await import("../../src/app/api/v1/[module]/[...path]/route");
  const response = await POST(
    new Request("http://localhost/api/v1/iam/users", { method: "POST", headers: { "X-Forwarded-For": "203.0.113.1" } }),
  );
  expect(response.status).toBe(405);
  expect(forwarded).toBeUndefined();
  expect(response.headers.get("Allow")).toBe("GET");
  expect(response.headers.get("Retry-After")).toBe("3");
  expect(response.headers.getSetCookie()).toHaveLength(2);
});
