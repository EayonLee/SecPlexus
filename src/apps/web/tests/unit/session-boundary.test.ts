import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { http } from "../../src/spx/api/client";
import { createUser, listUsers } from "../../src/spx/api/iam/management";
import { queryClient } from "../../src/spx/iam/query";
import { reauthenticate, refreshMe, useIdentity } from "../../src/spx/iam/session";
import { createServer, type ServerResponse } from "node:http";

let grants = ["iam.users.read"];
let session = "old-session";
let delayed: ServerResponse | undefined;
let arrived: (() => void) | undefined;
let reauthRejected = false;
let writeReply: { status: number; contentType: string; body: string } | undefined;
const identity = () => ({
  principal: {
    user_id: "user",
    session_id: session,
    username: "admin",
    display_name: "Admin",
    configured: true,
    auth_epoch: "1",
    authenticated_at: "2026-10-29T08:30:00Z",
  },
  roles: [{ id: "role", name: "Super administrator" }],
  permissions: grants,
});
const server = createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.method === "GET" && request.url?.startsWith("/api/v1/iam/users")) {
    delayed = response;
    arrived?.();
    return;
  }
  if (request.method === "POST" && request.url === "/api/v1/iam/users" && writeReply) {
    response.setHeader("Content-Type", writeReply.contentType);
    response.statusCode = writeReply.status;
    if (!writeReply.body) response.setHeader("Content-Length", "0");
    response.end(writeReply.body);
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
    response.end(
      JSON.stringify({
        code: "ok",
        message: "ok",
        request_id: "test-request",
        data: { principal: identity().principal },
      }),
    );
    return;
  }
  if (request.url === "/api/v1/iam/auth/context") {
    response.end(
      JSON.stringify({ code: "ok", message: "ok", request_id: "test-request", data: { csrf_token: "test-token" } }),
    );
    return;
  }
  if (request.url === "/api/v1/iam/me") {
    response.end(JSON.stringify({ code: "ok", message: "ok", request_id: "test-request", data: identity() }));
    return;
  }
  response.writeHead(404);
  response.end();
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

test("accepting a changed permission set removes old cached projections", async () => {
  await refreshMe();
  expect(useIdentity.getState().status).toBe("authenticated");
  expect(useIdentity.getState().permissions).toEqual(["iam.users.read"]);
  queryClient.setQueryData(["old-projection"], { restricted: true });
  grants = ["iam.users.read", "iam.roles.read"];
  await refreshMe();
  expect(queryClient.getQueryData(["old-projection"])).toBeUndefined();
  expect(useIdentity.getState().permissions).toEqual(["iam.users.read", "iam.roles.read"]);
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

test("confirming identity keeps the current page mounted while replacing its session scope", async () => {
  const states: string[] = [];
  const unsubscribe = useIdentity.subscribe((state) => states.push(state.status));
  await reauthenticate("test-password");
  unsubscribe();
  expect(states).not.toContain("unknown");
  expect(useIdentity.getState().permissions).toEqual(["iam.users.read", "iam.roles.read"]);
  expect(useIdentity.getState().status).toBe("authenticated");
});

const validCreated = JSON.stringify({ code: "ok", message: "ok", request_id: "test", data: { id: "target" } });
const createBody = { username: "new_analyst", display_name: "New analyst", password: "123456789012" };
for (const reply of [
  { status: 200, contentType: "text/html", body: "<html>Gateway sign in</html>" },
  { status: 201, contentType: "text/html", body: "<html>Gateway sign in</html>" },
  { status: 201, contentType: "application/json", body: "" },
  { status: 201, contentType: "application/json", body: "{}" },
  { status: 204, contentType: "application/json", body: "" },
  { status: 200, contentType: "application/json", body: validCreated },
])
  test(`invalid write acknowledgement rejects without mutation success: ${reply.status}/${reply.contentType}/${reply.body.length}`, async () => {
    writeReply = reply;
    let acknowledged = false;
    const mutation = queryClient.getMutationCache().build(queryClient, {
      mutationFn: createUser,
      onSuccess: () => {
        acknowledged = true;
      },
    });
    await expect(mutation.execute(createBody)).rejects.toBeDefined();
    expect(acknowledged).toBe(false);
  });

test("JSON acknowledgements are validated even if the media type is missing or incorrect", async () => {
  writeReply = { status: 201, contentType: "text/plain", body: validCreated };
  const response = await createUser(createBody);
  expect(response.id).toBe("target");
});
