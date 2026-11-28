import { expect, test } from "vitest";

import { readWebConfiguration } from "@/config/environment.mjs";

const environment = {
  SPX_WEB_PORT: "15173",
  SPX_IAM_HOST: "10.0.0.12",
  SPX_IAM_PORT: "18081",
  SPX_CONSOLE_HOST: "console.corp",
  SPX_CONSOLE_PORT: "18080",
};

test("remote service locations do not depend on an external browser origin", () => {
  expect(readWebConfiguration(environment)).toEqual({
    webPort: 15173,
    bindHost: "0.0.0.0",
    apiOrigins: { iam: "http://10.0.0.12:18081", console: "http://console.corp:18080" },
  });
});

test("missing service locations and unusable addresses are rejected", () => {
  expect(() => readWebConfiguration({ ...environment, SPX_IAM_HOST: undefined })).toThrow();
  expect(() => readWebConfiguration({ ...environment, SPX_IAM_HOST: "0.0.0.0" })).toThrow();
  expect(() => readWebConfiguration({ ...environment, SPX_IAM_PORT: "65536" })).toThrow();
});

test("Ingest upstream follows the configured runtime target", () => {
  expect(
    readWebConfiguration({ ...environment, SPX_INGEST_HOST: "ingest.corp", SPX_INGEST_PORT: "18082" }).apiOrigins,
  ).toMatchObject({ ingest: "http://ingest.corp:18082" });
  expect(
    readWebConfiguration({
      ...environment,
      SPX_INGEST_ORIGIN: "https://ingest.corp:18082",
      SPX_INGEST_HOST: "0.0.0.0",
      SPX_INGEST_PORT: "0",
    }).apiOrigins,
  ).toMatchObject({ ingest: "https://ingest.corp:18082" });
  expect(() => readWebConfiguration({ ...environment, SPX_INGEST_ORIGIN: "http://invalid.corp/path" })).toThrow(
    /SPX_INGEST_ORIGIN/,
  );
});

test("configuration diagnostics name the environment key without exposing its value", () => {
  let message = "";
  try {
    readWebConfiguration({ ...environment, SPX_IAM_PORT: "private-invalid-value" });
  } catch (error) {
    message = String(error);
  }
  expect(message).toContain("SPX_IAM_PORT");
  expect(message).not.toContain("private-invalid-value");
});
