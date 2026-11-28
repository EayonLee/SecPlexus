import { spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { integrationEnvironment } from './dependencies.mjs';
import { kafkaContainer } from './kafka-container.mjs';

// External runs select only test endpoints. Docker provisioning is opt-in.
const useDocker = process.argv.includes('--docker');
const withClickHouse = process.argv.includes('--clickhouse');
const owned = [];
let child;
const startup = new AbortController();
function interrupt(signal) {
  startup.abort(new Error('Integration run interrupted'));
  child?.kill(signal);
}
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, interrupt);
function docker(args, env = process.env) {
  const result = spawnSync('docker', args, { env, encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`Docker operation failed: ${args[0]}`);
  return result.stdout.trim();
}
try {
  const env = integrationEnvironment({ useDocker, withClickHouse });
  if (useDocker) {
    const name = `spx-test-${randomBytes(6).toString('hex')}`;
    env.POSTGRES_PASSWORD = randomBytes(24).toString('hex');
    const pg = `${name}-pg`;
    const redis = `${name}-redis`;
    docker(['run', '-d', '--name', pg, '-e', 'POSTGRES_PASSWORD', '-p', '127.0.0.1::5432', '--health-cmd', 'pg_isready -U postgres', '--health-interval', '1s', '--health-retries', '30', 'postgres:16-alpine'], env);
    owned.push(pg);
    docker(['run', '-d', '--name', redis, '-p', '127.0.0.1::6379', 'redis:7-alpine']);
    owned.push(redis);
    const kafka = `${name}-kafka`;
    const broker = await kafkaContainer(kafka);
    docker(broker.args);
    owned.push(kafka);
    const healthChecks = [pg, kafka];
    if (withClickHouse) {
      const clickhouse = `${name}-clickhouse`;
      const serverConfig = fileURLToPath(new URL('./clickhouse-test.xml', import.meta.url));
      docker(['run', '-d', '--name', clickhouse, '-e', 'CLICKHOUSE_SKIP_USER_SETUP=1',
        '-p', '127.0.0.1::8123', '-v', `${serverConfig}:/etc/clickhouse-server/config.d/spx-test.xml:ro`,
        '--health-cmd', 'clickhouse-client --query "SELECT 1"', '--health-interval', '1s',
        '--health-retries', '45', 'clickhouse/clickhouse-server:26.8.47.5']);
      owned.push(clickhouse);
      healthChecks.push(clickhouse);
      const port = docker(['port', clickhouse, '8123/tcp']).split(':').at(-1);
      env.SPX_TEST_CLICKHOUSE_URL = `http://127.0.0.1:${port}`;
      env.SPX_TEST_CLICKHOUSE_BACKUPS = '1';
    }
    const deadline = Date.now() + 90000;
    for (const service of healthChecks) {
      while (docker(['inspect', '--format', '{{.State.Health.Status}}', service]) !== 'healthy') {
        startup.signal.throwIfAborted();
        if (Date.now() > deadline) throw new Error('Test service did not become healthy: ' + service);
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    }
    env.SPX_TEST_KAFKA_BROKERS = broker.brokers;
    const pgPort = docker(['port', pg, '5432/tcp']).split(':').at(-1);
    const redisPort = docker(['port', redis, '6379/tcp']).split(':').at(-1);
    env.SPX_TEST_POSTGRESQL_CONTAINER = pg;
    env.SPX_TEST_POSTGRESQL_URL = `postgres://postgres:${env.POSTGRES_PASSWORD}@127.0.0.1:${pgPort}/postgres?sslmode=disable`;
    env.SPX_TEST_REDIS_URL = `redis://127.0.0.1:${redisPort}/0`;
  } else {
    for (const tool of ['pg_dump', 'pg_restore']) {
      if (spawnSync(tool, ['--version'], { env, stdio: 'ignore' }).status !== 0)
        throw new Error(`External PostgreSQL integration tests require ${tool} on PATH. Use client tools matching the server major version.`);
    }
  }
  if (withClickHouse && !env.SPX_TEST_CLICKHOUSE_URL) {
    throw new Error('ClickHouse integration requires SPX_TEST_CLICKHOUSE_URL or --docker.');
  }
  if (withClickHouse) env.SPX_TEST_REQUIRE_CLICKHOUSE = '1';
  startup.signal.throwIfAborted();
  child = spawn('go', ['test', ...(process.argv.includes('--race') ? ['-race'] : []), '-tags', 'integration', '-count=1', './src/apps/iam/...', './src/apps/console/...', './src/apps/ingest/...'], { env, stdio: 'inherit' });
  process.exitCode = await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code) => resolve(code ?? 1));
  });
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  for (const name of owned.reverse()) {
    const result = spawnSync('docker', ['rm', '-f', '-v', name], { stdio: 'ignore' });
    if (result.status !== 0) { console.error(`Could not remove test container ${name}`); process.exitCode = 1; }
  }
  for (const signal of ['SIGINT', 'SIGTERM']) process.removeListener(signal, interrupt);
}
