import { afterAll, afterEach, beforeAll, beforeEach, expect, test } from "vitest";

import { client } from "../../src/spx/api/iam/client.gen";
import { changePassword } from "../../src/spx/api/iam/sdk.gen";
import { APIError, mutationHeaders, useIdentity } from "../../src/spx/iam/session";
import { initialAuth } from "../../src/spx/iam/types";
import { createServer } from "node:http";

const nativeFetch = globalThis.fetch;
const server = createServer((_request, response) => {
  response.writeHead(503, { "Content-Type": "application/json" });
  response.end(
    JSON.stringify({ code: "service_unavailable", message: "服务暂时不可用", data: null, request_id: "test-request" }),
  );
});
let origin: string;
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("Missing isolated HTTP listener");
  origin = `http://127.0.0.1:${address.port}`;
});
beforeEach(() => {
  // Node测试用自己的HTTP监听器解析浏览器相对URL，响应仍通过真实fetch传输。
  client.setConfig({ baseUrl: origin, fetch: nativeFetch });
  useIdentity.setState(
    {
      ...initialAuth,
      status: "authenticated",
      principal: {
        user_id: "user",
        session_id: "session",
        username: "member",
        display_name: "Member",
        is_super_admin: false,
        auth_version: "1",
        authenticated_at: "2026-10-28T00:00:00Z",
      },
    },
    true,
  );
});
afterEach(() => {
  globalThis.fetch = nativeFetch;
});
afterAll(async () => {
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});

test("CSRF bootstrap failure pauses authenticated actions without logging the user out", async () => {
  await expect(
    changePassword({
      headers: mutationHeaders,
      body: { old_password: "old password", new_password: "new password" },
      throwOnError: true,
    }),
  ).rejects.toBeInstanceOf(APIError);
  expect(useIdentity.getState().status).toBe("unavailable");
  expect(useIdentity.getState().principal?.user_id).toBe("user");
});
