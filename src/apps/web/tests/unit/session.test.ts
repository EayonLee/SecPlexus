import { afterAll, beforeAll, beforeEach, expect, test } from "vitest";

import { APIError, http } from "../../src/spx/api/client";
import { changePassword } from "../../src/spx/api/iam/auth";
import { useIdentity } from "../../src/spx/iam/session";
import { initialAuth } from "../../src/spx/iam/types";
import { createServer } from "node:http";

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
  // Node 测试使用自有 HTTP 监听器，仍由 Axios 真实传输。
  http.defaults.baseURL = origin;
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
afterAll(async () => {
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});

test("CSRF bootstrap failure pauses authenticated actions without logging the user out", async () => {
  await expect(
    changePassword({
      old_password: "old password",
      new_password: "new password",
    }),
  ).rejects.toBeInstanceOf(APIError);
  expect(useIdentity.getState().status).toBe("unavailable");
  expect(useIdentity.getState().principal?.user_id).toBe("user");
});
