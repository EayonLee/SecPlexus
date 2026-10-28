import { spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';

// Docker deployment is explicit. An externally provisioned test endpoint can be
// supplied instead; no development or production configuration is loaded here.
const useDocker = process.argv.includes('--docker');
const owned = [];
function docker(args, env = process.env) {
  const result = spawnSync('docker', args, { env, encoding: 'utf8' });
  if (result.status !== 0) throw new Error(`Docker operation failed: ${args[0]}`);
  return result.stdout.trim();
}
try {
  const env = { ...process.env };
  if (useDocker) {
    const name = `spx-test-${randomBytes(6).toString('hex')}`;
    env.POSTGRES_PASSWORD = randomBytes(24).toString('hex');
    const pg = `${name}-pg`;
    const redis = `${name}-redis`;
    docker(['run', '-d', '--name', pg, '-e', 'POSTGRES_PASSWORD', '-p', '127.0.0.1::5432', '--health-cmd', 'pg_isready -U postgres', '--health-interval', '1s', '--health-retries', '30', 'postgres:16-alpine'], env);
    owned.push(pg);
    docker(['run', '-d', '--name', redis, '-p', '127.0.0.1::6379', 'redis:7-alpine']);
    owned.push(redis);
    const deadline = Date.now() + 60000;
    while (docker(['inspect', '--format', '{{.State.Health.Status}}', pg]) !== 'healthy') {
      if (Date.now() > deadline) throw new Error('Test PostgreSQL did not become healthy');
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
    const pgPort = docker(['port', pg, '5432/tcp']).split(':').at(-1);
    const redisPort = docker(['port', redis, '6379/tcp']).split(':').at(-1);
    env.SPX_TEST_POSTGRES_CONTAINER = pg;
    env.SPX_TEST_POSTGRES_URL = `postgres://postgres:${env.POSTGRES_PASSWORD}@127.0.0.1:${pgPort}/postgres?sslmode=disable`;
    env.SPX_TEST_REDIS_URL = `redis://127.0.0.1:${redisPort}/0`;
  }
  if (!env.SPX_TEST_POSTGRES_URL || !env.SPX_TEST_REDIS_URL) {
    throw new Error('Provide SPX_TEST_POSTGRES_URL and SPX_TEST_REDIS_URL, or explicitly pass --docker.');
  }
  const child = spawn('go', ['test', ...(process.argv.includes('--race') ? ['-race'] : []), '-tags', 'integration', '-count=1', './src/apps/iam/...', './src/apps/console/...'], { env, stdio: 'inherit' });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => child.kill(signal));
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
}
