import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { http } from "../../src/spx/api/client";
import { updateUser } from "../../src/spx/api/iam/management";
import { managementChanged } from "../../src/spx/iam/management";
import { queryClient } from "../../src/spx/iam/query";
import { useIdentity } from "../../src/spx/iam/session";
import { createServer } from "node:http";

const server = createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.method === "PATCH" && request.url === "/api/v1/iam/users/target") {
    response.writeHead(204);
    response.end();
    return;
  }
  if (request.method !== "GET" || !["/api/v1/iam/auth/context", "/api/v1/iam/me"].includes(request.url ?? "")) {
    response.writeHead(404);
    response.end();
    return;
  }
  const data =
    request.url === "/api/v1/iam/auth/context"
      ? { csrf_token: "isolated-test-token" }
      : {
          principal: {
            user_id: "user",
            session_id: "session",
            username: "member",
            display_name: "Member",
            is_super_admin: true,
            auth_version: "1",
            authenticated_at: "2026-10-29T08:30:00Z",
          },
          organizations: [],
          capabilities: [],
          platform_capabilities: [],
        };
  response.end(JSON.stringify({ code: "ok", message: "ok", request_id: "test-request", data }));
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing owned test listener");
  const origin = `http://127.0.0.1:${address.port}`;
  http.defaults.baseURL = origin;
  vi.stubGlobal("window", { location: { origin, pathname: "/dashboard", search: "" } });
});
afterAll(async () => {
  queryClient.clear();
  vi.unstubAllGlobals();
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});
test("a management mutation invalidates all cached pages and related projections", async () => {
  const keys = [
    [{ _id: "listUsers" }, "session", 1],
    [{ _id: "listUsers" }, "session", 2],
    [{ _id: "listRoles" }, "org", "version"],
    [{ _id: "getOrganization" }, "org", "version"],
  ];
  for (const key of keys) queryClient.setQueryData(key, { previous: "projection" });
  const mutation = queryClient.getMutationCache().build(queryClient, {
    mutationFn: updateUser,
    onSuccess: managementChanged,
  });
  await mutation.execute({
    id: "target",
    body: { display_name: "Changed", status: "active", auth_version: "1" },
  });
  for (const key of keys) expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
  expect(useIdentity.getState().status).toBe("authenticated");
});
