import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { loadEnvironment, webEnvironment } from './environment.mjs';
const root = fileURLToPath(new URL('..', import.meta.url));
// Builds can use defaults without a development .env; existing configuration
// still controls public values that Next.js freezes into its production output.
const environment = existsSync(path.join(root, '.env')) ? loadEnvironment(root) : process.env;
mkdirSync(path.join(root, 'bin'), { recursive: true });
for (const [command, args] of [
  ['go', ['build', '-o', 'bin/iam', './src/apps/iam/cmd']],
  ['go', ['build', '-o', 'bin/console', './src/apps/console/cmd']],
  ['pnpm', ['--dir', 'src/apps/web', 'build']],
]) {
  const result = spawnSync(command, args, { cwd: root, env: command === 'pnpm' ? webEnvironment(environment) : process.env, stdio: 'inherit' });
  if (result.error || result.status !== 0) { process.exitCode = 1; break; }
}
