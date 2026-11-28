import { spawn, spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadEnvironment, moduleEnvironment } from './environment.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const action = process.argv[2] ?? 'check-store-config';
try {
  if (process.argv.length > 3 || !['check-store-config', 'init-event-topics', 'migrate-event-store', 'check-event-store', 'store-events'].includes(action)) {
    throw new Error('Use check-store-config, init-event-topics, migrate-event-store, check-event-store or store-events.');
  }
  const env = moduleEnvironment(loadEnvironment(root), 'ingest');
  const directory = mkdtempSync(path.join(tmpdir(), 'spx-event-store-'));
  try {
    const executable = path.join(directory, process.platform === 'win32' ? 'ingest.exe' : 'ingest');
    const built = spawnSync('go', ['build', '-o', executable, './src/apps/ingest/cmd'], { cwd: root, stdio: 'inherit' });
    if (built.status !== 0) throw new Error('Could not build the Ingest event storage command.');

    const child = spawn(executable, [action, '--config', 'src/apps/ingest/config.yaml'], { cwd: root, env, stdio: 'inherit' });
    const stop = () => child.kill('SIGTERM');
    process.once('SIGINT', stop);
    process.once('SIGTERM', stop);
    try {
      process.exitCode = await new Promise((resolve, reject) => {
        child.once('error', reject);
        child.once('exit', (code) => resolve(code ?? 1));
      });
    } finally {
      process.removeListener('SIGINT', stop);
      process.removeListener('SIGTERM', stop);
    }
  } finally {
    rmSync(directory, { recursive: true, force: true, maxRetries: 3 });
  }
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
