import { spawn, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { constants, copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync, watch, writeFileSync } from "node:fs";
import net from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../../../../", import.meta.url));
process.chdir(root);

const systemEnv = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !key.startsWith("SPX_") && !key.startsWith("NEXT_PUBLIC_")),
);
let ownsRootEnv = false;
let ownedDatabase;
let postgresAdminURL;
const directory = mkdtempSync(path.join(tmpdir(), "spx-e2e-"));
const name = `spx-e2e-${randomBytes(5).toString("hex")}`;
const containers = [];
const children = [];
const listenerPorts = [];
let stackReady = false;
function command(binary, args, options = {}) {
  const result = spawnSync(binary, args, { encoding: "utf8", ...options });
  if (result.status !== 0) throw new Error(`${binary} ${args[0]} failed; no credentials are logged.`);
  return result.stdout?.trim();
}
async function port() {
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const number = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return number;
}
function launch(binary, args, env) {
  const child = spawn(binary, args, { env, stdio: ["ignore", "pipe", "pipe"], detached: process.platform !== "win32" });
  child.stdout.resume();
  child.stderr.resume();
  children.push(child);
  return child;
}
async function ready(url, child) {
  const deadline = Date.now() + 60000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`Test process exited before readiness: ${new URL(url).pathname}`);
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      /* 下一次就绪检查仍受总截止时间约束。 */
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`Test process readiness timeout: ${new URL(url).pathname}`);
}
try {
  if (!existsSync(".env")) {
    try {
      copyFileSync(".env.example", ".env", constants.COPYFILE_EXCL);
      ownsRootEnv = true;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
    }
  }
  if (!process.argv.includes("--docker") && (!process.env.SPX_TEST_POSTGRES_URL || !process.env.SPX_TEST_REDIS_URL))
    throw new Error("Provide SPX_TEST_POSTGRES_URL and SPX_TEST_REDIS_URL, or explicitly pass --docker.");
  const pgPassword = randomBytes(24).toString("hex");
  const password = randomBytes(24).toString("hex");
  const adminUsername = `e2e_${randomBytes(8).toString("hex")}`;
  const credential = randomBytes(32).toString("hex");
  const env = { ...systemEnv, POSTGRES_PASSWORD: pgPassword };
  const pg = `${name}-pg`;
  const redis = `${name}-redis`;
  let databaseURL;
  let redisURL;
  if (process.argv.includes("--docker")) {
    const dockerDatabase = `spx_e2e_${randomBytes(16).toString("hex")}`;
    command(
      "docker",
      [
        "run",
        "-d",
        "--name",
        pg,
        "-e",
        "POSTGRES_PASSWORD",
        "-e",
        `POSTGRES_DB=${dockerDatabase}`,
        "-p",
        "127.0.0.1::5432",
        "--health-cmd",
        "pg_isready -U postgres",
        "--health-interval",
        "1s",
        "--health-retries",
        "30",
        "postgres:16-alpine",
      ],
      { env },
    );
    containers.push(pg);
    command("docker", ["run", "-d", "--name", redis, "-p", "127.0.0.1::6379", "redis:7-alpine"]);
    containers.push(redis);
    const deadline = Date.now() + 60000;
    while (command("docker", ["inspect", "--format", "{{.State.Health.Status}}", pg]) !== "healthy") {
      if (Date.now() > deadline) throw new Error("Test database readiness timeout");
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
    const pgPort = command("docker", ["port", pg, "5432/tcp"]).split(":").at(-1);
    const redisPort = command("docker", ["port", redis, "6379/tcp"]).split(":").at(-1);
    databaseURL = `postgres://postgres:${pgPassword}@127.0.0.1:${pgPort}/${dockerDatabase}?sslmode=disable`;
    redisURL = `redis://127.0.0.1:${redisPort}/0`;
  } else {
    postgresAdminURL = process.env.SPX_TEST_POSTGRES_URL;
    ownedDatabase = command("go", ["run", "./tests/integration/database", "create"], {
      env: { ...systemEnv, SPX_TEST_POSTGRES_URL: postgresAdminURL },
    });
    const parsed = new URL(postgresAdminURL);
    parsed.pathname = `/${ownedDatabase}`;
    databaseURL = parsed.toString();
    redisURL = process.env.SPX_TEST_REDIS_URL;
  }
  const webPort = await port();
  const iamPort = await port();
  const consolePort = await port();
  const iamPort2 = await port();
  const consolePort2 = await port();
  listenerPorts.push(webPort, iamPort, consolePort, iamPort2, consolePort2);
  const publicURL = `http://127.0.0.1:${webPort}`;
  const csrf = randomBytes(16).toString("hex");
  const iamBinary = path.join(directory, "iam");
  const consoleBinary = path.join(directory, "console");
  command("go", ["build", "-o", iamBinary, "./src/apps/iam/cmd"]);
  command("go", ["build", "-o", consoleBinary, "./src/apps/console/cmd"]);
  command(iamBinary, ["migrate", "--config", "src/apps/iam/config.yaml"], {
    env: {
      ...systemEnv,
      SPX_POSTGRES_URL: databaseURL,
      SPX_IAM_BOOTSTRAP_PASSWORD: password,
      SPX_IAM_BOOTSTRAP_USERNAME: adminUsername,
      SPX_IAM_LOG_FILE: "",
    },
  });
  const iamEnv = {
    ...systemEnv,
    SPX_WEB_ORIGIN: publicURL,
    SPX_POSTGRES_URL: databaseURL,
    SPX_REDIS_URL: redisURL,
    SPX_IAM_POSTGRES_URL: databaseURL,
    SPX_IAM_REDIS_URL: redisURL,
    SPX_IAM_BOOTSTRAP_USERNAME: adminUsername,
    SPX_IAM_BOOTSTRAP_PASSWORD: password,
    SPX_IAM_CSRF_KEY: csrf,
    SPX_CONSOLE_IAM_TOKEN: credential,
    SPX_IAM_LOG_FILE: path.join(directory, "iam-one.log"),
  };
  const consoleEnv = {
    ...systemEnv,
    SPX_CONSOLE_IAM_TOKEN: credential,
    SPX_CONSOLE_LOG_FILE: path.join(directory, "console-one.log"),
  };
  const rootEnv = {
    ...iamEnv,
    ...consoleEnv,
    SPX_IAM_HOST: "127.0.0.1",
    SPX_IAM_PORT: String(iamPort),
    SPX_CONSOLE_HOST: "127.0.0.1",
    SPX_CONSOLE_PORT: String(consolePort),
    SPX_WEB_BIND_HOST: "127.0.0.1",
    SPX_WEB_PORT: String(webPort),
    SPX_IAM_BIND_PORT: String(iamPort),
    SPX_CONSOLE_BIND_PORT: String(consolePort),
    SPX_IAM_BIND_HOST: "127.0.0.1",
    SPX_CONSOLE_BIND_HOST: "127.0.0.1",
  };
  // Root startup derives backend URLs from actual module ports. Use one DB
  // connection for initialization, HTTP replicas and maintenance.
  const mode = process.argv.includes("--dev") ? "dev" : "preview";
  // Preview 使用构建产物；dev 复用同一入口验证原生热更新。
  // 构建环境不带数据库密码及内部调用凭证。
  if (mode === "preview")
    command("pnpm", ["--dir", "src/apps/web", "build"], {
      env: { ...systemEnv, SPX_WEB_ORIGIN: publicURL, SPX_WEB_PORT: String(webPort), SPX_IAM_HOST:"127.0.0.1",SPX_IAM_PORT:String(iamPort),SPX_CONSOLE_HOST:"127.0.0.1",SPX_CONSOLE_PORT:String(consolePort) },
    });
  const stack = launch(process.execPath, ["scripts/start.mjs", mode], rootEnv);
  await ready(`${publicURL}/login`, stack);
  await ready(`${publicURL}/api/v1/iam/auth/context`, stack);
  const iam2 = launch(iamBinary, ["serve", "--config", "src/apps/iam/config.yaml"], {
    ...iamEnv,
    SPX_IAM_HOST: "127.0.0.1",
    SPX_IAM_PORT: String(iamPort2),
    SPX_IAM_BIND_HOST: "127.0.0.1",
    SPX_IAM_LOG_FILE: path.join(directory, "iam-two.log"),
  });
  await ready(`http://127.0.0.1:${iamPort2}/health/ready`, iam2);
  const console2 = launch(consoleBinary, ["--config", "src/apps/console/config.yaml"], {
    ...consoleEnv,
    SPX_CONSOLE_HOST: "127.0.0.1",
    SPX_CONSOLE_PORT: String(consolePort2),
    SPX_CONSOLE_BIND_HOST: "127.0.0.1",
    SPX_IAM_HOST: "127.0.0.1",
    SPX_IAM_PORT: String(iamPort2),
    SPX_CONSOLE_LOG_FILE: path.join(directory, "console-two.log"),
  });
  await ready(`http://127.0.0.1:${consolePort2}/health/ready`, console2);
  stackReady = true;
  if (process.argv.includes("--manual")) {
    const filename = path.join(directory, "browser.json");
    writeFileSync(
      filename,
      JSON.stringify({
        baseURL: publicURL,
        database: ownedDatabase ?? new URL(databaseURL).pathname.slice(1),
        iamSecondURL: `http://127.0.0.1:${iamPort2}`,
        consoleSecondURL: `http://127.0.0.1:${consolePort2}`,
        username: adminUsername,
        password: password,
        iamLog: path.join(directory, "iam-one.log"),
      }),
      { mode: 0o600 },
    );
    console.log(`Isolated browser acceptance ready: ${filename}`);
    console.log("Create stop in the same temporary directory, send input, or interrupt to clean up the owned stack.");
    await new Promise((resolve) => {
      const stop = path.join(directory, "stop");
      const finish = () => {
        observer.close();
        process.stdin.removeListener("data", finish);
        process.removeListener("SIGINT", finish);
        process.removeListener("SIGTERM", finish);
        process.stdin.pause();
        resolve();
      };
      const observer = watch(directory, () => {
        if (existsSync(stop)) finish();
      });
      process.stdin.once("data", finish);
      process.once("SIGINT", finish);
      process.once("SIGTERM", finish);
    });
  } else {
    const child = spawn(
      "pnpm",
      [
        "--dir",
        "src/apps/web",
        "exec",
        "playwright",
        "test",
        ...process.argv.slice(2).filter((arg) => arg !== "--docker" && arg !== "--dev"),
      ],
      {
        env: {
          ...systemEnv,
          SPX_E2E_BASE_URL: publicURL,
          SPX_E2E_DATABASE: ownedDatabase ?? new URL(databaseURL).pathname.slice(1),
          SPX_TEST_POSTGRES_URL: databaseURL,
          SPX_E2E_PASSWORD: password,
          SPX_E2E_USERNAME: adminUsername,
          SPX_E2E_IAM_SECOND_URL: `http://127.0.0.1:${iamPort2}`,
          SPX_E2E_CONSOLE_SECOND_URL: `http://127.0.0.1:${consolePort2}`,
        },
        stdio: "inherit",
      },
    );
    for (const signal of ["SIGINT", "SIGTERM"]) process.once(signal, () => child.kill(signal));
    process.exitCode = await new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("exit", (code) => resolve(code ?? 1));
    });
  }
  // Exercise the actual Console SSE handler while shutting down its owned replica.
  const { request } = await import("@playwright/test");
  const api = await request.newContext({ baseURL: publicURL });
  try {
    const context = await api.get("/api/v1/iam/auth/context");
    const token = (await context.json()).data.csrf_token;
    const authenticated = await api.post("/api/v1/iam/auth/login", {
      data: { username: adminUsername, password: password },
      headers: { Origin: publicURL, "X-CSRF-Token": token },
    });
    if (authenticated.status() !== 200) throw new Error("Cannot establish the owned lifecycle test session.");
    const identity = (await (await api.get("/api/v1/iam/me")).json()).data;
    const org = identity.organizations[0].id;
    const cookies = (await api.storageState()).cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
    const stream = await fetch(`http://127.0.0.1:${consolePort2}/api/v1/console/orgs/${org}/events`, {
      headers: { Cookie: cookies, Origin: publicURL },
      signal: AbortSignal.timeout(8000),
    });
    if (stream.status !== 200 || !stream.body) throw new Error("Owned Console event stream did not open.");
    const reader = stream.body.getReader();
    const first = await reader.read();
    if (!new TextDecoder().decode(first.value).includes("authorized"))
      throw new Error("Console stream did not authorize.");
    const closed = new Promise((resolve) => console2.once("exit", (code, signal) => resolve({ code, signal })));
    console2.kill("SIGTERM");
    while (!(await reader.read()).done) {
      /* Read through the final event until normal EOF. */
    }
    const exit = await closed;
    if (exit.code !== 0 || exit.signal) throw new Error("Console needed forced shutdown.");
    console.log("Actual Console SSE observed cancellation and ended before normal process exit.");
  } finally {
    await api.dispose();
  }
  for (const filename of ["iam-one.log", "iam-two.log", "console-one.log", "console-two.log"]) {
    const data = readFileSync(path.join(directory, filename), "utf8");
    if (![pgPassword, password, credential, csrf].every((value) => !data.includes(value)))
      throw new Error("A secret reached application logs.");
    if (
      !data
        .trim()
        .split("\n")
        .every((line) => JSON.parse(line).service)
    )
      throw new Error("Application logs must carry structured service metadata.");
  }
  command(iamBinary, ["maintain", "--config", "src/apps/iam/config.yaml"], {
    env: { ...systemEnv, SPX_POSTGRES_URL: databaseURL, SPX_IAM_LOG_FILE: "" },
  });
  console.log("Structured file logs and secret exclusion verified across both replicas.");
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  for (const child of children.reverse()) {
    if (child.exitCode !== null || child.signalCode !== null) continue;
    try {
      if (process.platform === "win32") child.kill("SIGTERM");
      else process.kill(-child.pid, "SIGTERM");
    } catch (error) {
      if (error.code !== "ESRCH") process.exitCode = 1;
    }
    await Promise.race([
      new Promise((resolve) => {
        if (child.exitCode !== null) resolve();
        else child.once("exit", resolve);
      }),
      new Promise((resolve) => setTimeout(resolve, 5000)),
    ]);
    if (child.exitCode === null && child.signalCode === null) {
      console.error("An owned service required forced shutdown.");
      process.exitCode = 1;
      try {
        process.kill(-child.pid, "SIGKILL");
      } catch {
        /* 下一次就绪检查仍受总截止时间约束。 */
      }
    }
  }
  if (stackReady)
    for (const number of listenerPorts) {
      const listener = net.createServer();
      await new Promise((resolve, reject) => {
        listener.once("error", reject);
        listener.listen(number, "127.0.0.1", resolve);
      });
      await new Promise((resolve, reject) => listener.close((error) => (error ? reject(error) : resolve())));
    }
  if (ownedDatabase) {
    command("go", ["run", "./tests/integration/database", "drop", ownedDatabase], {
      env: {
        ...systemEnv,
        SPX_TEST_POSTGRES_URL: postgresAdminURL,
        SPX_TEST_REDIS_URL: process.env.SPX_TEST_REDIS_URL,
      },
    });
  }
  for (const container of containers.reverse()) {
    const result = spawnSync("docker", ["rm", "-f", "-v", container], { stdio: "ignore" });
    if (result.status !== 0) {
      console.error(`Cleanup failed for ${container}`);
      process.exitCode = 1;
    }
  }
  rmSync(directory, { recursive: true, force: true });
  if (ownsRootEnv) rmSync(".env", { force: true });
}
