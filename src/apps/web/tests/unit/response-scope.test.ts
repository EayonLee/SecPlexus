import { afterAll, beforeAll, expect, test } from "vitest";
import { z } from "zod";

import { http, request } from "../../src/spx/api/client";
import { advanceRequestScope } from "../../src/spx/iam/request-scope";
import { createServer } from "node:http";

const server = createServer((_request, response) => {
  response.setHeader("Content-Type", "application/json");
  response.end(
    JSON.stringify({ code: "ok", message: "ok", request_id: "owned-request", data: { value: "old identity" } }),
  );
});
beforeAll(async () => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("Missing owned listener");
  http.defaults.baseURL = `http://127.0.0.1:${address.port}`;
});
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
});
test("identity changes during asynchronous response validation reject the already received data", async () => {
  let entered: () => void = () => undefined;
  let release: () => void = () => undefined;
  const parsing = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  const schema = z.object({ value: z.string() }).refine(async () => {
    entered();
    await waiting;
    return true;
  });
  const outcome = request({ url: "/owned-response" }, schema).then(
    () => "unexpected success",
    (error: unknown) => error,
  );
  await parsing;
  advanceRequestScope();
  release();
  expect(await outcome).toMatchObject({ name: "AbortError" });
});
