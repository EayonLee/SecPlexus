import { spawn, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { testDependencies } from "../../../../../tests/integration/dependencies.mjs";
import { kafkaContainer } from "../../../../../tests/integration/kafka-container.mjs";
import { constants, copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync, watch, writeFileSync } from "node:fs";
import net from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { assertStackAvailable } from "../../../../../scripts/runtime.mjs";
import { terminateProcessTree } from "../../../../../scripts/processes.mjs";

const root = fileURLToPath(new URL("../../../../../", import.meta.url));
const executableSuffix = process.platform === "win32" ? ".exe" : "";
const pnpmShell = process.platform === "win32" ? { shell: true } : {};
process.chdir(root);

const systemEnv = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !key.startsWith("SPX_") && !key.startsWith("NEXT_PUBLIC_")),
);
let ownsRootEnv = false;
let ownedDatabase;
const useDocker = process.argv.includes("--docker");
const withClickHouse = process.argv.includes("--clickhouse");

let external;
let migratedDatabaseURL;
const kafkaNamespace = `spx-e2e-${randomBytes(16).toString("hex")}`;
const inputTopic = `${kafkaNamespace}.input`;
const eventTopic = `${kafkaNamespace}.ingest.events`;
const eventFailureTopic = `${kafkaNamespace}.ingest.event-failures`;
const eventStoreGroup = `${kafkaNamespace}-ingest-events`;
let eventStoreStarted = false;
const ownedTopics = [];
const directory = mkdtempSync(path.join(tmpdir(), "spx-e2e-"));
const name = `spx-e2e-${randomBytes(5).toString("hex")}`;
const kafkaAdmin = path.join(directory, `kafka-admin${executableSuffix}`);
function kafkaOperation(action, name, brokers = external?.SPX_TEST_KAFKA_BROKERS) {
  const result = spawnSync(kafkaAdmin, [action, name], {
    env: { ...systemEnv, SPX_TEST_KAFKA_BROKERS: brokers },
    stdio: "inherit",
  });
  if (result.status !== 0) throw new Error(`Kafka ${action} failed for ${name}`);
}
const containers = [];
const children = [];
// 收尾失败时保留工作目录，供事后读日志；成功则照常清理。
let keepDirectory = false;
const listenerPorts = [];
// 阶段标记：e2e 卡住时不加这个，只能看到一个笼统的终止信号。
const startedAt = Date.now();
function stage(label) {
  console.error(`[e2e +${String(Date.now() - startedAt).padStart(6)}ms] ${label}`);
}
let stackReady = false;
// 启动阶段也要进入 finally；此时可能已经建好外部数据库和 Kafka 主题。
const startup = new AbortController();
const interruptStartup = () => startup.abort(new Error("E2E startup interrupted"));
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, interruptStartup);
function command(binary, args, options = {}) {
  const result = spawnSync(binary, args, { encoding: "utf8", ...(binary === "pnpm" ? pnpmShell : {}), ...options });
  if (result.status !== 0) throw new Error(`${binary} ${args[0]} failed; no credentials are logged.`);
  return result.stdout?.trim();
}
async function port() {
  startup.signal.throwIfAborted();
  const server = net.createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const number = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return number;
}
// 子进程输出保留尾部：启动失败时不回显日志，就只能看到"未就绪"这种没有信息量的报错。
const outputTails = new WeakMap();
const tailLimit = 4000;
function launch(binary, args, env, options = {}) {
  const child = spawn(binary, args, {
    env,
    stdio: ["ignore", "pipe", "pipe"],
    detached: options.detached ?? process.platform !== "win32",
  });
  child.spxName = options.name ?? path.basename(binary);
  let output = "";
  const collect = (chunk) => {
    output += chunk.toString();
    if (output.length > tailLimit) output = output.slice(-tailLimit);
  };
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  outputTails.set(child, () => output);
  children.push(child);
  return child;
}
function tailOf(child) {
  const read = outputTails.get(child);
  return read ? read().trim() : "";
}
async function ready(url, child) {
  const deadline = Date.now() + 60000;
  while (Date.now() < deadline) {
    startup.signal.throwIfAborted();
    if (child.exitCode !== null) {
      const tail = tailOf(child);
      throw new Error(
        `Test process exited before readiness: ${new URL(url).pathname}` +
          (tail ? `\n--- child output ---\n${tail}` : ""),
      );
    }
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      /* 下一次就绪检查仍受总截止时间约束。 */
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  const tail = tailOf(child);
  throw new Error(
    `Test process readiness timeout: ${new URL(url).pathname}` + (tail ? `\n--- child output ---\n${tail}` : ""),
  );
}
try {
  assertStackAvailable(root, true);
  if (!useDocker) external = testDependencies({ withClickHouse });
  if (withClickHouse && !external?.SPX_TEST_CLICKHOUSE_URL)
    throw new Error("Browser event storage verification requires a dedicated SPX_TEST_CLICKHOUSE_URL in external mode.");
  if (!existsSync(".env")) {
    try {
      copyFileSync(".env.example", ".env", constants.COPYFILE_EXCL);
      ownsRootEnv = true;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
    }
  }
  const pgPassword = randomBytes(24).toString("hex");
  const password = randomBytes(24).toString("hex");
  const adminUsername = `e2e_${randomBytes(8).toString("hex")}`;
  const credential = randomBytes(32).toString("hex");
  // 每个 service client 必须有独立凭证：共用一个会让授权身份不确定。
  const ingestCredential = randomBytes(32).toString("hex");
  const env = { ...systemEnv, POSTGRES_PASSWORD: pgPassword };
  const pg = `${name}-pg`;
  const redis = `${name}-redis`;
  let databaseURL;
  let redisURL;
  let kafkaBrokers = external?.SPX_TEST_KAFKA_BROKERS;
  if (useDocker) {
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
    // ingest 必须连得上总线：单节点 KRaft 足够，避免依赖 ZooKeeper。
    const kafka = `${name}-kafka`;
    const broker = await kafkaContainer(kafka);
    command("docker", broker.args);
    containers.push(kafka);
    const deadline = Date.now() + 60000;
    while (command("docker", ["inspect", "--format", "{{.State.Health.Status}}", pg]) !== "healthy") {
      startup.signal.throwIfAborted();
      if (Date.now() > deadline) throw new Error("Test database readiness timeout");
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
    const pgPort = command("docker", ["port", pg, "5432/tcp"]).split(":").at(-1);
    const redisPort = command("docker", ["port", redis, "6379/tcp"]).split(":").at(-1);
    databaseURL = `postgres://postgres:${pgPassword}@127.0.0.1:${pgPort}/${dockerDatabase}?sslmode=disable`;
    redisURL = `redis://127.0.0.1:${redisPort}/0`;
    kafkaBrokers = broker.brokers;
  } else {
    ownedDatabase = command("go", ["run", "./tests/integration/database", "create"], {
      env: { ...systemEnv, SPX_TEST_POSTGRESQL_URL: external.SPX_TEST_POSTGRESQL_URL },
    });
    const parsed = new URL(external.SPX_TEST_POSTGRESQL_URL);
    parsed.pathname = `/${ownedDatabase}`;
    databaseURL = parsed.toString();
    redisURL = external.SPX_TEST_REDIS_URL;
  }
  // 测试生产者与正式配置都依赖已创建的主题；Docker broker 也显式准备测试资源。
  command("go", ["build", "-o", kafkaAdmin, "./tests/integration/kafka-admin"]);
  if (useDocker) {
    const env = { ...systemEnv, SPX_TEST_KAFKA_BROKERS: kafkaBrokers };
    const deadline = Date.now() + 60000;
    while (spawnSync(kafkaAdmin, ["ready", "broker"], { env, stdio: "ignore" }).status !== 0) {
      startup.signal.throwIfAborted();
      if (Date.now() > deadline) throw new Error("Test Kafka readiness timeout");
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
  for (const topic of [inputTopic, eventTopic, ...(withClickHouse ? [eventFailureTopic] : [])]) {
    kafkaOperation("create-topic", topic, kafkaBrokers);
    if (!useDocker) ownedTopics.push(topic);
  }
  stage("test dependencies ready");
  const webPort = await port();
  const iamPort = await port();
  const consolePort = await port();
  const iamPort2 = await port();
  const consolePort2 = await port();
  const ingestPort = await port();
  listenerPorts.push(webPort, iamPort, consolePort, iamPort2, consolePort2, ingestPort);
  const publicURL = `http://127.0.0.1:${webPort}`;
  const csrf = randomBytes(16).toString("hex");
  const iamBinary = path.join(directory, `iam${executableSuffix}`);
  const consoleBinary = path.join(directory, `console${executableSuffix}`);
  command("go", ["build", "-o", iamBinary, "./src/apps/iam/cmd"]);
  command("go", ["build", "-o", consoleBinary, "./src/apps/console/cmd"]);
  const ingestBinary = path.join(directory, `ingest${executableSuffix}`);
  command("go", ["build", "-o", ingestBinary, "./src/apps/ingest/cmd"]);
  command(iamBinary, ["migrate", "--config", "src/apps/iam/config.yaml"], {
    env: { ...systemEnv, SPX_POSTGRESQL_URL: databaseURL, SPX_IAM_LOG_FILE: "" },
  });
  command(iamBinary, ["initialize-administrator", "--config", "src/apps/iam/config.yaml"], {
    env: {
      ...systemEnv,
      SPX_POSTGRESQL_URL: databaseURL,
      SPX_IAM_BOOTSTRAP_PASSWORD: password,
      SPX_IAM_BOOTSTRAP_USERNAME: adminUsername,
      SPX_IAM_LOG_FILE: "",
    },
  });
  const ingestEnv = {
    ...systemEnv,
    SPX_POSTGRESQL_URL: databaseURL,
    SPX_REDIS_URL: redisURL,
    SPX_INGEST_IAM_TOKEN: ingestCredential,
    SPX_INGEST_HOST: "127.0.0.1",
    SPX_INGEST_PORT: String(ingestPort),
    SPX_KAFKA_BROKERS: kafkaBrokers,
    SPX_INGEST_INPUT_KAFKA_RELOAD_INTERVAL: "1s",
    SPX_KAFKA_NAMESPACE: kafkaNamespace,
    SPX_INGEST_LOG_FILE: path.join(directory, "ingest.log"),
  };
  command(ingestBinary, ["migrate", "--config", "src/apps/ingest/config.yaml"], {
    env: { ...ingestEnv, SPX_INGEST_LOG_FILE: "" },
  });
  migratedDatabaseURL = databaseURL;
  command("go", ["run", "./tests/integration/database", "seed-ingest"], {
    env: { ...systemEnv, SPX_TEST_POSTGRESQL_URL: databaseURL },
  });
  const iamEnv = {
    ...systemEnv,
    SPX_WEB_ORIGIN: publicURL,
    SPX_POSTGRESQL_URL: databaseURL,
    SPX_REDIS_URL: redisURL,
    SPX_IAM_BOOTSTRAP_USERNAME: adminUsername,
    SPX_IAM_BOOTSTRAP_PASSWORD: password,
    SPX_IAM_CSRF_KEY: csrf,
    SPX_IAM_LOGIN_IP_RATE: "300",
    SPX_IAM_LOGIN_ACCOUNT_RATE: "200",
    SPX_CONSOLE_IAM_TOKEN: credential,
    // IAM 的 service_clients 含 ingest 条目：缺它的凭证时 IAM 拒绝启动。
    SPX_INGEST_IAM_TOKEN: ingestCredential,
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
    SPX_INGEST_HOST: "127.0.0.1",
    SPX_INGEST_PORT: String(ingestPort),
  };
  // Root startup derives backend URLs from actual module ports. Use one DB
  // connection for initialization, HTTP replicas and maintenance.
  const mode = process.argv.includes("--dev") ? "dev" : "preview";
  // Preview 使用构建产物；dev 复用同一入口验证原生热更新。
  // 构建环境不带数据库密码及内部调用凭证。
  if (mode === "preview")
    command("pnpm", ["--dir", "src/apps/web", "build"], {
      stdio: "inherit",
      env: {
        ...systemEnv,
        SPX_WEB_ORIGIN: publicURL,
        SPX_WEB_PORT: String(webPort),
        SPX_IAM_HOST: "127.0.0.1",
        SPX_IAM_PORT: String(iamPort),
        SPX_CONSOLE_HOST: "127.0.0.1",
        SPX_CONSOLE_PORT: String(consolePort),
      },
    });
  const stack = launch(process.execPath, ["scripts/start.mjs", mode, "iam", "console", "web"], rootEnv, {
    name: "stack",
  });
  await ready(`${publicURL}/login`, stack);
  await ready(`${publicURL}/api/v1/iam/auth/context`, stack);
  const iam2 = launch(
    iamBinary,
    ["serve", "--config", "src/apps/iam/config.yaml"],
    {
      ...iamEnv,
      SPX_IAM_HOST: "127.0.0.1",
      SPX_IAM_PORT: String(iamPort2),
      SPX_IAM_LOG_FILE: path.join(directory, "iam-two.log"),
    },
    { name: "iam-two" },
  );
  await ready(`http://127.0.0.1:${iamPort2}/health/ready`, iam2);
  const console2 = launch(
    consoleBinary,
    ["--config", "src/apps/console/config.yaml"],
    {
      ...consoleEnv,
      SPX_CONSOLE_HOST: "127.0.0.1",
      SPX_CONSOLE_PORT: String(consolePort2),
      SPX_IAM_HOST: "127.0.0.1",
      SPX_IAM_PORT: String(iamPort2),
      SPX_CONSOLE_LOG_FILE: path.join(directory, "console-two.log"),
    },
    { name: "console-two" },
  );
  await ready(`http://127.0.0.1:${consolePort2}/health/ready`, console2);
  // 测试单独管理 Ingest 的 Kafka 主题和端口，因此根启动入口只负责其余三个模块。
  const ingest = launch(
    ingestBinary,
    ["serve", "--config", "src/apps/ingest/config.yaml"],
    {
      ...ingestEnv,
      SPX_IAM_HOST: "127.0.0.1",
      SPX_IAM_PORT: String(iamPort),
    },
    { name: "ingest" },
  );
  await ready(`http://127.0.0.1:${ingestPort}/health/ready`, ingest);
  if (withClickHouse) {
    const eventStoreEnv = {
      ...ingestEnv,
      SPX_CLICKHOUSE_URL: external.SPX_TEST_CLICKHOUSE_URL,
      SPX_INGEST_LOG_FILE: path.join(directory, "event-store.log"),
    };
    command(ingestBinary, ["migrate-event-store", "--config", "src/apps/ingest/config.yaml"], { env: eventStoreEnv });
    launch(ingestBinary, ["store-events", "--config", "src/apps/ingest/config.yaml"], eventStoreEnv, { name: "event-store" });
    eventStoreStarted = true;
  }
  const permissionCatalog = await fetch(`http://127.0.0.1:${iamPort}/api/v1/iam/permission-catalog`, {
    headers: { Authorization: `Bearer ${ingestCredential}` },
    signal: startup.signal,
  });
  if (!permissionCatalog.ok)
    throw new Error(`Ingest could not read the IAM permission catalog: ${permissionCatalog.status}.`);
  const permissionKeys = (await permissionCatalog.json()).data?.keys;
  if (!permissionKeys?.includes("ingest.parsing_rule.read"))
    throw new Error("IAM permission catalog does not include ingest.parsing_rule.read.");
  startup.signal.throwIfAborted();
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
    stage("playwright:start");
    const child = spawn(
      "pnpm",
      [
        "--dir",
        "src/apps/web",
        "exec",
        "playwright",
        "test",
        ...process.argv.slice(2).filter((arg) => arg !== "--docker" && arg !== "--dev" && arg !== "--clickhouse"),
      ],
      {
        ...pnpmShell,
        env: {
          ...systemEnv,
          SPX_E2E_BASE_URL: publicURL,
          SPX_E2E_DATABASE: ownedDatabase ?? new URL(databaseURL).pathname.slice(1),
          SPX_TEST_POSTGRESQL_URL: databaseURL,
          SPX_E2E_PASSWORD: password,
          SPX_E2E_USERNAME: adminUsername,
          SPX_E2E_IAM_SECOND_URL: `http://127.0.0.1:${iamPort2}`,
          SPX_E2E_CONSOLE_SECOND_URL: `http://127.0.0.1:${consolePort2}`,
          SPX_E2E_INGEST_URL: `http://127.0.0.1:${ingestPort}`,
          SPX_E2E_ROOT: root,
          SPX_E2E_KAFKA_BROKERS: kafkaBrokers,
          SPX_E2E_EVENT_TOPIC: eventTopic,
          SPX_E2E_INPUT_TOPIC: inputTopic,
          ...(withClickHouse ? { SPX_E2E_CLICKHOUSE_URL: external.SPX_TEST_CLICKHOUSE_URL } : {}),
        },
        stdio: "inherit",
      },
    );
    for (const signal of ["SIGINT", "SIGTERM"])
      process.once(signal, () => {
        if (process.platform === "win32" && child.pid)
          spawnSync("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore" });
        else child.kill(signal);
      });
    process.exitCode = await new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("exit", (code) => resolve(code ?? 1));
    });
    stage(`playwright:exit code=${process.exitCode}`);
  }
  stage("console:authorization");
  const { request } = await import("@playwright/test");
  const api = await request.newContext({ baseURL: publicURL });
  try {
    const context = await api.get("/api/v1/iam/auth/context");
    const token = (await context.json()).data.csrf_token;
    const authenticated = await api.post("/api/v1/iam/auth/login", {
      data: { username: adminUsername, password },
      headers: { Origin: publicURL, "X-CSRF-Token": token },
    });
    if (authenticated.status() !== 200) throw new Error("Cannot establish the Console lifecycle test session.");

    const cookies = (await api.storageState()).cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
    const consoleContext = `http://127.0.0.1:${consolePort2}/api/v1/console/context`;
    const headers = { Cookie: cookies, Origin: publicURL };
    if ((await api.get(consoleContext, { headers })).status() !== 200)
      throw new Error("Console did not authorize the active session.");

    const authenticatedContext = await api.get("/api/v1/iam/auth/context");
    const logout = await api.post("/api/v1/iam/auth/logout", {
      headers: { Origin: publicURL, "X-CSRF-Token": (await authenticatedContext.json()).data.csrf_token },
    });
    if (!logout.ok()) throw new Error(`Could not revoke the Console test session: ${logout.status()}`);
    if ((await api.get(consoleContext, { headers })).status() !== 401)
      throw new Error("Console accepted the revoked session.");
  } finally {
    await api.dispose();
  }
  stage("logs:scan");
  for (const filename of ["iam-one.log", "iam-two.log", "console-one.log", "console-two.log", "ingest.log"]) {
    const data = readFileSync(path.join(directory, filename), "utf8");
    if (![pgPassword, password, credential, ingestCredential, csrf].every((value) => !data.includes(value)))
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
    env: { ...systemEnv, SPX_POSTGRESQL_URL: databaseURL, SPX_IAM_LOG_FILE: "" },
  });
  console.log("Structured file logs and secret exclusion verified across both replicas.");

  // ingest 关停要自己走完：缓冲里可能还有事件要发出去。
  // 只靠收尾统一终止时，卡住也只能看到"某个服务被强制终止"，定位不到是谁。
  const ingestClosed = new Promise((resolve) => ingest.once("exit", (code, signal) => resolve({ code, signal })));
  if (process.platform === "win32") ingest.kill();
  else ingest.kill("SIGTERM");
  const ingestExit = await Promise.race([
    ingestClosed,
    new Promise((resolve) => setTimeout(() => resolve({ code: null, signal: "timeout" }), 15000)),
  ]);
  if (ingestExit.signal === "timeout") {
    // 卡住时要指出停在哪一步，否则只能靠猜。
    const tail = tailOf(ingest);
    let logTail = "";
    try {
      logTail = readFileSync(path.join(directory, "ingest.log"), "utf8").trim().split("\n").slice(-6).join("\n");
    } catch {
      // 日志文件可能还没落盘：诊断信息缺失不应掩盖原始失败。
      logTail = "";
    }
    throw new Error(
      `Ingest did not stop within the shutdown budget.\nstdout/stderr tail:\n${tail}\nlog tail:\n${logTail}`,
    );
  }
  console.log(`Ingest stopped with its own shutdown path (code=${ingestExit.code}).`);
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  async function cleanup(action) {
    try {
      await action();
    } catch (error) {
      console.error(error.message);
      process.exitCode = 1;
    }
  }
  for (const child of children.reverse()) {
    if (child.exitCode !== null || child.signalCode !== null) continue;
    await cleanup(async () => {
      await terminateProcessTree(child);
      if (child.exitCode === null && child.signalCode === null)
        throw new Error(`An owned service required forced shutdown: ${child.spxName}.`);
    });
  }
  if (stackReady)
    for (const number of listenerPorts) {
      await cleanup(async () => {
        const listener = net.createServer();
        await new Promise((resolve, reject) => {
          listener.once("error", reject);
          listener.listen(number, "127.0.0.1", resolve);
        });
        await new Promise((resolve, reject) => listener.close((error) => (error ? reject(error) : resolve())));
      });
    }
  if (external && migratedDatabaseURL) {
    await cleanup(async () => {
      const groups = command("go", ["run", "./tests/integration/database", "kafka-groups", inputTopic], {
        env: { ...systemEnv, SPX_TEST_POSTGRESQL_URL: migratedDatabaseURL },
      });
      for (const group of groups.split(/\s+/).filter(Boolean))
        await cleanup(() => kafkaOperation("delete-group", group));
    });
  }
  if (eventStoreStarted) await cleanup(() => kafkaOperation("delete-group", eventStoreGroup));
  for (const topic of ownedTopics) await cleanup(() => kafkaOperation("delete-topic", topic));
  if (ownedDatabase) {
    await cleanup(() =>
      command("go", ["run", "./tests/integration/database", "drop", ownedDatabase], {
        env: { ...systemEnv, ...external },
      }),
    );
  }
  for (const container of containers.reverse()) {
    const result = spawnSync("docker", ["rm", "-f", "-v", container], { stdio: "ignore" });
    if (result.status !== 0) {
      console.error(`Cleanup failed for ${container}`);
      process.exitCode = 1;
    }
  }
  if (!keepDirectory) rmSync(directory, { recursive: true, force: true });
  if (ownsRootEnv) rmSync(".env", { force: true });
  for (const signal of ["SIGINT", "SIGTERM"]) process.removeListener(signal, interruptStartup);
}
