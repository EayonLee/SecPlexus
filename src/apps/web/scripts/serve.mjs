import nextEnv from "@next/env";

import { readWebConfiguration } from "../src/config/environment.mjs";
import { spawn, spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const directory = fileURLToPath(new URL("..", import.meta.url));
const command = process.argv[2];
try {
  if (!["dev", "start", "check-config"].includes(command)) throw new Error("Use dev, start or check-config");
  nextEnv.loadEnvConfig(
    directory,
    command === "dev" || (command === "check-config" && process.argv.includes("--development")),
  );
  const configuration = readWebConfiguration(process.env);
  if (command === "check-config") {
    console.log("Web configuration valid");
  } else {
    const require = createRequire(import.meta.url);
    const child = spawn(
      process.execPath,
      [
        require.resolve("next/dist/bin/next"),
        command,
        "--hostname",
        configuration.bindHost,
        "--port",
        String(configuration.webPort),
        ...process.argv.slice(3),
      ],
      { cwd: directory, env: process.env, stdio: "inherit" },
    );
    for (const signal of ["SIGINT", "SIGTERM"]) {
      process.once(signal, () => {
        // Windows 只结束 Next CLI 会留下它启动的服务进程，用 /T 一并结束。
        if (process.platform === "win32" && child.pid) {
          spawnSync("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore" });
        } else {
          child.kill(signal);
        }
      });
    }
    child.once("error", () => {
      console.error("Cannot start Next.js");
      process.exitCode = 1;
    });
    child.once("exit", (code, signal) => {
      process.exitCode = code ?? (signal ? 0 : 1);
    });
  }
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
