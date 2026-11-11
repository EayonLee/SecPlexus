import { spawnSync } from "node:child_process";
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// sqlc vet 的 db-prepare 规则会把每条查询在真实数据库上 PREPARE 一次，
// 抓出静态分析看不出的类型与转换问题。脚本自己建库、跑真实迁移、结束时删库，
// 因此既能本地跑，也能在 CI 里跑；连接串只从环境读取，不落盘、不打印。
const root = fileURLToPath(new URL("..", import.meta.url));
process.chdir(root);

const adminURL = process.env.SPX_TEST_POSTGRES_URL;
if (!adminURL) {
  console.error("SPX_TEST_POSTGRES_URL is required: sqlc vet prepares every query against a real database.");
  process.exit(1);
}

const sqlcVersion = process.env.SQLC_VERSION ?? "v1.31.1";
const gooseVersion = process.env.GOOSE_VERSION ?? "v3.29.0";
// 配置必须与被检查的 schema 同目录解析相对路径，因此放在仓库根目录，用完即删。
const configPath = ".sqlc-vet.yaml";

function run(command, args, extraEnv = {}) {
  const result = spawnSync(command, args, {
    env: { ...process.env, ...extraEnv },
    stdio: ["ignore", "pipe", "inherit"],
    encoding: "utf8",
  });
  if (result.status !== 0) throw new Error(`${command} ${args[0]} failed`);
  return result.stdout?.trim() ?? "";
}

function vetConfig() {
  const source = readFileSync("sqlc.yaml", "utf8");
  const block = [
    "    database:",
    '      uri: "${SQLC_VET_URL}"',
    "    rules:",
    "      - sqlc/db-prepare",
    "",
  ].join("\n");
  const needle = "    queries: src/apps/iam/internal/repository/queries\n";
  if (!source.includes(needle)) throw new Error("sqlc.yaml no longer matches the expected layout");
  writeFileSync(configPath, source.replace(needle, needle + block), "utf8");
}

let database;
try {
  database = run("go", ["run", "./tests/integration/database", "create"]);
  const target = new URL(adminURL);
  target.pathname = `/${database}`;
  const dsn = target.toString();

  run("go", [
    "run",
    `github.com/pressly/goose/v3/cmd/goose@${gooseVersion}`,
    "-dir",
    "src/apps/iam/migrations",
    "postgres",
    dsn,
    "up",
  ]);
  console.log("Migrations applied to the owned vet database.");

  vetConfig();
  run("go", ["run", `github.com/sqlc-dev/sqlc/cmd/sqlc@${sqlcVersion}`, "vet", "-f", configPath], {
    SQLC_VET_URL: dsn,
  });
  console.log("Every generated query prepared successfully.");
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  rmSync(configPath, { force: true });
  if (database) {
    const result = spawnSync("go", ["run", "./tests/integration/database", "drop", database], {
      stdio: ["ignore", "ignore", "inherit"],
      env: process.env,
    });
    if (result.status !== 0) {
      console.error("Could not remove the owned vet database.");
      process.exitCode = 1;
    }
  }
}
