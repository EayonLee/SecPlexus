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
const directory = mkdtempSync(path.join(tmpdir(), "spx-e2e-"));
const name = `spx-e2e-${randomBytes(5).toString("hex")}`;
const containers = [];
const children = [];
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
  if (!process.argv.includes("--docker"))
    throw new Error("Pass --docker to explicitly provision the isolated full-stack browser environment.");
  const pgPassword = randomBytes(24).toString("hex");
  const initialPassword = "secplexus";
  const password = randomBytes(24).toString("hex");
  const credential = randomBytes(32).toString("hex");
  const env = { ...systemEnv, POSTGRES_PASSWORD: pgPassword };
  const pg = `${name}-pg`;
  const redis = `${name}-redis`;
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
      "POSTGRES_DB=secplexus",
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
  const webPort = await port();
  const iamPort = await port();
  const internalPort = await port();
  const consolePort = await port();
  const iamPort2 = await port();
  const internalPort2 = await port();
  const consolePort2 = await port();
  const publicURL = `http://127.0.0.1:${webPort}`;
  const databaseURL = `postgres://postgres:${pgPassword}@127.0.0.1:${pgPort}/secplexus?sslmode=disable`;
  const csrf = randomBytes(16).toString("hex");
  const cert = path.join(directory, "cert.pem");
  const key = path.join(directory, "key.pem");
  command("openssl", [
    "req",
    "-x509",
    "-newkey",
    "rsa:2048",
    "-nodes",
    "-keyout",
    key,
    "-out",
    cert,
    "-subj",
    "/CN=localhost",
    "-addext",
    "subjectAltName=IP:127.0.0.1,DNS:localhost",
    "-days",
    "1",
  ]);
  const iamBinary = path.join(directory, "iam");
  const consoleBinary = path.join(directory, "console");
  command("go", ["build", "-o", iamBinary, "./src/apps/iam/cmd"]);
  command("go", ["build", "-o", consoleBinary, "./src/apps/console/cmd"]);
  command(iamBinary, ["migrate", "--config", "src/apps/iam/config.yaml"], {
    env: {
      ...systemEnv,
      SPX_POSTGRES_URL: databaseURL,
      SPX_IAM_BOOTSTRAP_PASSWORD: initialPassword,
      SPX_IAM_LOG_FILE: "",
    },
  });
  const iamEnv = {
    ...systemEnv,
    SPX_PUBLIC_URL: publicURL,
    SPX_POSTGRES_URL: databaseURL,
    SPX_REDIS_URL: `redis://127.0.0.1:${redisPort}/0`,
    SPX_IAM_CSRF_KEY: csrf,
    SPX_CONSOLE_IAM_TOKEN: credential,
    SPX_IAM_TLS_CERT_FILE: cert,
    SPX_IAM_TLS_KEY_FILE: key,
    SPX_IAM_LOG_FILE: path.join(directory, "iam-one.log"),
  };
  const consoleEnv = {
    ...systemEnv,
    SPX_CONSOLE_IAM_TOKEN: credential,
    SPX_CONSOLE_IAM_CA_FILE: cert,
    SPX_CONSOLE_LOG_FILE: path.join(directory, "console-one.log"),
  };
  const rootEnv = {
    ...iamEnv,
    ...consoleEnv,
    SPX_IAM_HOST: "127.0.0.1",
    SPX_IAM_PORT: String(iamPort),
    SPX_IAM_INTERNAL_HOST: "127.0.0.1",
    SPX_IAM_INTERNAL_PORT: String(internalPort),
    SPX_CONSOLE_HOST: "127.0.0.1",
    SPX_CONSOLE_PORT: String(consolePort),
    SPX_WEB_HOST: "127.0.0.1",
    SPX_WEB_PORT: String(webPort),
  };
  // Root startup derives backend URLs from actual module ports. Use one DB
  // connection for initialization, HTTP replicas and maintenance.
  rootEnv.SPX_IAM_BOOTSTRAP_PASSWORD = "";
  rootEnv.SPX_CONSOLE_IAM_BASE_URL = "";
  rootEnv.SPX_WEB_IAM_PROXY_TARGET = "";
  rootEnv.SPX_WEB_CONSOLE_PROXY_TARGET = "";
  // 使用本地构建产物验证完整流程，避免开发路由在测试中动态编译。
  // 构建环境不带数据库密码及内部调用凭证。
  command("pnpm", ["--dir", "src/apps/web", "build"], {
    env: { ...systemEnv, NEXT_PUBLIC_APP_URL: publicURL },
  });
  const stack = launch(process.execPath, ["scripts/start.mjs", "preview"], rootEnv);
  await ready(`${publicURL}/login`, stack);
  await ready(`${publicURL}/api/v1/iam/auth/context`, stack);
  const iam2 = launch(iamBinary, ["serve", "--config", "src/apps/iam/config.yaml"], {
    ...iamEnv,
    SPX_IAM_HOST: "127.0.0.1",
    SPX_IAM_PORT: String(iamPort2),
    SPX_IAM_INTERNAL_HOST: "127.0.0.1",
    SPX_IAM_INTERNAL_PORT: String(internalPort2),
    SPX_IAM_LOG_FILE: path.join(directory, "iam-two.log"),
  });
  await ready(`http://127.0.0.1:${iamPort2}/health/ready`, iam2);
  const console2 = launch(consoleBinary, ["--config", "src/apps/console/config.yaml"], {
    ...consoleEnv,
    SPX_CONSOLE_HOST: "127.0.0.1",
    SPX_CONSOLE_PORT: String(consolePort2),
    SPX_CONSOLE_IAM_BASE_URL: `https://127.0.0.1:${internalPort2}`,
    SPX_CONSOLE_LOG_FILE: path.join(directory, "console-two.log"),
  });
  await ready(`http://127.0.0.1:${consolePort2}/health/ready`, console2);
  if(process.argv.includes("--manual")){
   const filename=path.join(directory,"browser.json");
   writeFileSync(filename,JSON.stringify({baseURL:publicURL,username:"admin",password:initialPassword}),{mode:0o600});
   console.log(`Isolated browser acceptance ready: ${filename}`);
   console.log("Create stop in the same temporary directory, send input, or interrupt to clean up the owned stack.");
   await new Promise(resolve=>{
    const stop=path.join(directory,"stop");
    const finish=()=>{observer.close();process.stdin.removeListener("data",finish);process.removeListener("SIGINT",finish);process.removeListener("SIGTERM",finish);process.stdin.pause();resolve()};
    const observer=watch(directory,()=>{if(existsSync(stop))finish()});
    process.stdin.once("data",finish);
    process.once("SIGINT",finish);
    process.once("SIGTERM",finish);
   });
  }else{
  const child = spawn(
    "pnpm",
    [
      "--dir",
      "src/apps/web",
      "exec",
      "playwright",
      "test",
      ...process.argv.slice(2).filter((arg) => arg !== "--docker"),
    ],
    {
      env: {
        ...systemEnv,
        SPX_E2E_BASE_URL: publicURL,
        SPX_E2E_INITIAL_PASSWORD: initialPassword,
        SPX_E2E_PASSWORD: password,
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
  for (const filename of ["iam-one.log", "iam-two.log", "console-one.log", "console-two.log"]) {
    const data = readFileSync(path.join(directory, filename), "utf8");
    if (![pgPassword, initialPassword, password, credential, csrf].every((value) => !data.includes(value)))
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
    if (child.exitCode === null) {
      try {
        process.kill(-child.pid, "SIGKILL");
      } catch {
        /* 下一次就绪检查仍受总截止时间约束。 */
      }
    }
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
