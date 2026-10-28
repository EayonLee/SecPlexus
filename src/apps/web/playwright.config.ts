import { defineConfig, devices } from "@playwright/test";

if (!process.env.SPX_E2E_BASE_URL)
  throw new Error("Run the isolated E2E entrypoint; no default developer server is used.");
export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 60000,
  expect: { timeout: 10000 },
  reporter: [["list"]],
  use: {
    actionTimeout: 10000,
    baseURL: process.env.SPX_E2E_BASE_URL,
    trace: "off",
    video: "off",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 960 } } }],
});
