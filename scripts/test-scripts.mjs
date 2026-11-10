import { readdirSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const files = readdirSync(path.join(root, 'scripts'))
  .filter((name) => name.endsWith('.test.mjs'))
  .sort()
  .map((name) => path.join(root, 'scripts', name));
if (!files.length) throw new Error('No script tests found.');
const result = spawnSync(process.execPath, ['--test', ...process.argv.slice(2), ...files], {
  cwd: root,
  stdio: 'inherit',
});
process.exitCode = result.status ?? 1;
