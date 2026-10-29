import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { http } from "../../src/spx/api/client";
import { createOrganization, listUsers } from "../../src/spx/api/iam/management";
import { queryClient } from "../../src/spx/iam/query";
import { reauthenticate, refreshIdentity, useIdentity } from "../../src/spx/iam/session";
import { createServer, type ServerResponse } from "node:http";

const org = "09aa1d46-7c4b-493a-aef4-2e6e9b6c93ab";
let version = "1";
let session = "old-session";
let delayed: ServerResponse | undefined;
let arrived: (() => void) | undefined;
let reauthRejected = false;
let organizationReply: { status: number; contentType: string; body: string } | undefined;
const server = createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.url?.startsWith("/api/v1/iam/users")) {
    delayed = response;
    arrived?.();
    return;
  }
  if (request.url === "/api/v1/iam/auth/reauthenticate") {
    if (reauthRejected) {
      response
        .writeHead(400)
        .end(
          JSON.stringify({ code: "iam.invalid_current_password", message: "invalid", data: null, request_id: "test" }),
        );
      return;
    }
    session = "new-session";
  }
  if (request.url === "/api/v1/iam/organizations" && organizationReply) {
    response.setHeader("Content-Type", organizationReply.contentType);
    response.statusCode = organizationReply.status;
    if (!organizationReply.body) response.setHeader("Content-Length", "0");
    response.end(organizationReply.body);
    return;
  }
  const data =
    request.url === "/api/v1/iam/auth/context"
      ? { csrf_token: "test-token" }
      : {
          principal: {
            user_id: "user",
            session_id: session,
            username: "admin",
            display_name: "Admin",
            is_super_admin: true,
            auth_version: "1",
            authenticated_at: "2026-10-29T08:30:00Z",
          },
          organizations: [{ id: org, name: "Ops", slug: "ops", status: "active", authz_version: version }],
          org_id: org,
          capabilities: [],
          platform_capabilities: [],
        };
  response.end(
    JSON.stringify({
      code: "ok",
      message: "ok",
      request_id: "test-request",
      data:
        request.url === "/api/v1/iam/auth/reauthenticate" && "principal" in data ? { principal: data.principal } : data,
    }),
  );
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing test listener");
  const origin = `http://127.0.0.1:${address.port}`;
  http.defaults.baseURL = origin;
  vi.stubGlobal("window", { location: { origin, pathname: "/dashboard", search: "" } });
});
afterAll(async () => {
  queryClient.clear();
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
  vi.unstubAllGlobals();
});

test("accepting a changed authorization scope removes old cached projections", async () => {
  await refreshIdentity(org);
  expect(useIdentity.getState().status).toBe("authenticated");
  queryClient.setQueryData(["old-projection"], { restricted: true });
  version = "2";
  await refreshIdentity(org);
  expect(queryClient.getQueryData(["old-projection"])).toBeUndefined();
});

test("a late unauthorized response cannot erase the reauthenticated identity", async () => {
  const waiting = new Promise<void>((resolve) => {
    arrived = resolve;
  });
  const old = listUsers().catch((error: unknown) => error);
  await waiting;
  await reauthenticate("test-password");
  expect(useIdentity.getState().principal?.session_id).toBe("new-session");
  delayed
    ?.writeHead(401)
    .end(JSON.stringify({ code: "unauthenticated", message: "expired", data: null, request_id: "old" }));
  await old;
  expect(useIdentity.getState().status).toBe("authenticated");
  expect(useIdentity.getState().principal?.session_id).toBe("new-session");
});

test("a rejected password confirmation preserves the current session", async () => {
  const previous = useIdentity.getState().principal?.session_id;
  reauthRejected = true;
  await expect(reauthenticate("wrong-password")).rejects.toBeDefined();
  expect(useIdentity.getState().status).toBe("authenticated");
  expect(useIdentity.getState().principal?.session_id).toBe(previous);
  reauthRejected = false;
});

const validCreated = JSON.stringify({ code: "ok", message: "ok", request_id: "test", data: { id: org } });
for (const reply of [
  { status: 200, contentType: "text/html", body: "<html>Gateway sign in</html>" },
  { status: 201, contentType: "text/html", body: "<html>Gateway sign in</html>" },
  { status: 201, contentType: "application/json", body: "" },
  { status: 201, contentType: "application/json", body: "{}" },
  { status: 204, contentType: "application/json", body: "" },
  { status: 200, contentType: "application/json", body: validCreated },
])
  test(`invalid write acknowledgement rejects without mutation success: ${reply.status}/${reply.contentType}/${reply.body.length}`, async () => {
    organizationReply = reply;
    let acknowledged = false;
    const mutation = queryClient.getMutationCache().build(queryClient, {
      mutationFn: createOrganization,
      onSuccess: () => {
        acknowledged = true;
      },
    });
    await expect(mutation.execute({ slug: "ops", name: "Ops", admin_user_id: org })).rejects.toBeDefined();
    expect(acknowledged).toBe(false);
  });

test("JSON acknowledgements are validated even if the media type is missing or incorrect", async () => {
  organizationReply = { status: 201, contentType: "text/plain", body: validCreated };
  const response = await createOrganization({
    slug: "ops",
    name: "Ops",
    admin_user_id: org,
  });
  expect(response.id).toBe(org);
});
