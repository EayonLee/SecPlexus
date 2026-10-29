import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadEnvironment } from './environment.mjs';
import { webListener } from './endpoints.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
try {
  const env = loadEnvironment(root);
  // Internal Compose substitution only; the public URL remains the user setting.
  env.SPX_COMPOSE_WEB_PORT = String(webListener(env).port);
  const checkout = createHash('sha256').update(root).digest('hex').slice(0, 8);
  const services = process.argv.slice(2);
  if (services.some((service) => !['postgres', 'redis', 'iam-migrate', 'iam', 'console', 'web'].includes(service))) {
    throw new Error('Unknown Compose service name.');
  }
  const result = spawnSync('docker', [
    'compose', '--project-directory', root, '--env-file', '.env',
    '--project-name', env.COMPOSE_PROJECT_NAME || `spx-${checkout}`,
    '-f', 'deploy/compose/compose.yaml', 'up', '-d', '--build', '--wait', '--wait-timeout', '120', ...services,
  ], { cwd: root, env, stdio: 'inherit' });
  if (result.error) throw new Error('Cannot run Docker Compose. Check that Docker and Compose are available.');
  process.exitCode = result.status ?? 1;
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
