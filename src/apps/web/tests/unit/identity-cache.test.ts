import { afterAll, afterEach, beforeAll, expect, test, vi } from "vitest";

import { http } from "../../src/spx/api/client";
import { me } from "../../src/spx/api/iam/auth";
import { queryClient } from "../../src/spx/iam/query";
import { signOut, useIdentity } from "../../src/spx/iam/session";
import { createServer } from "node:http";

let entered: () => void;
let release: () => void;
const waiting = new Promise<void>((resolve) => {
  release = resolve;
});
const server = createServer(async (request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.url === "/api/v1/iam/auth/context") {
    response.end(
      JSON.stringify({ code: "ok", message: "ok", request_id: "request", data: { csrf_token: "owned-test-token" } }),
    );
    return;
  }
  if (request.url === "/api/v1/iam/auth/logout") {
    response.writeHead(204);
    response.end();
    return;
  }
  if (request.url === "/api/v1/iam/me") {
    entered();
    await waiting;
    if (!response.destroyed)
      response.end(JSON.stringify({ code: "ok", message: "ok", request_id: "request", data: {} }));
    return;
  }
  response.writeHead(404);
  response.end();
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing isolated test listener");
  const origin = `http://127.0.0.1:${address.port}`;
  http.defaults.baseURL = origin;
  vi.stubGlobal("window", { location: { origin, pathname: "/dashboard", search: "" } });
});
afterEach(() => {
  release();
  queryClient.clear();
  vi.unstubAllGlobals();
});
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});

test("logout cancels in-flight requests and removes the previous identity's cached data", async () => {
  useIdentity.setState({ status: "authenticated" });
  queryClient.setQueryData(["owned-private-query"], { value: "previous identity" });
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const outcome = me().then(
    () => "unexpected success",
    (error: unknown) => error,
  );
  await started;
  await signOut();
  release();
  expect(await outcome).toMatchObject({ name: "AbortError" });
  expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
  expect(useIdentity.getState().status).toBe("anonymous");
});
