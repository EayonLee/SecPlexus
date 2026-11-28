import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { createServer } from "node:http";

let forwarded: string | undefined;
let authorization: string | undefined;
let edgeCredential: string | undefined;
const edgeToken = "a".repeat(64);
const server = createServer((request, response) => {
  forwarded = request.headers["x-forwarded-for"] as string | undefined;
  authorization = request.headers.authorization;
  edgeCredential = request.headers["x-spx-edge-token"] as string | undefined;
  if (request.url?.startsWith("/api/v1/ingest/")) {
    response.writeHead(401, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ code: "ingest.unauthorized" }));
    return;
  }
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
  vi.stubEnv("SPX_INGEST_HOST", "127.0.0.1");
  vi.stubEnv("SPX_INGEST_PORT", String(address.port));
  vi.stubEnv("SPX_WEB_PORT", "15173");
  vi.stubEnv("SPX_WEB_EDGE_TOKEN", edgeToken);
});

test("Ingest forwarding preserves device authentication and the upstream response", async () => {
  const { GET, POST } = await import("../../src/app/api/v1/[module]/[...path]/route");
  const headers = { authorization: "Bearer test-device-token", "content-type": "text/plain" };
  const device = await POST(
    new Request("http://localhost/api/v1/ingest/events/device-id", { method: "POST", headers, body: "fixture" }),
  );
  expect(device.status).toBe(401);
  expect(await device.json()).toEqual({ code: "ingest.unauthorized" });
  expect(authorization).toBe(headers.authorization);
  const management = await GET(new Request("http://localhost/api/v1/ingest/inputs", { headers }));
  expect(management.status).toBe(401);
  expect(authorization).toBeUndefined();
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

test("rejects a login body over the limit without a content length", async () => {
  const { POST } = await import("../../src/app/api/v1/[module]/[...path]/route");
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array(2048));
      controller.enqueue(new Uint8Array(1));
      controller.close();
    },
  });
  const init = { method: "POST", body, duplex: "half" } as const;
  const response = await POST(new Request("http://localhost/api/v1/iam/auth/login", init));
  expect(response.status).toBe(413);
  expect(await response.json()).toMatchObject({ code: "payload_too_large" });
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

test("forwards only the single address supplied by the configured HTTPS entry", async () => {
  const { GET } = await import("../../src/app/api/v1/[module]/[...path]/route");
  for (const [token, address, expected] of [
    [edgeToken, "2001:db8::5", "2001:db8::5"],
    ["b".repeat(64), "192.0.2.5", undefined],
    [edgeToken, "192.0.2.5, 192.0.2.6", undefined],
  ]) {
    await GET(new Request("http://localhost/api/v1/iam/users", {
      headers: {
        "X-SPX-Edge-Token": token!,
        "X-SPX-Client-IP": address!,
        "X-Forwarded-For": "203.0.113.1",
      },
    }));
    expect(forwarded).toBe(expected);
    expect(edgeCredential).toBeUndefined();
  }
});
