import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, mkdirSync, renameSync, rmSync, watch } from 'node:fs';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { loadEnvironment, moduleEnvironment, webEnvironment } from './environment.mjs';
import { endpoint, consoleEndpoint, webEndpoints, webListener } from './endpoints.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const children = new Set();
const intentional = new Set();
const watchers = [];
let stopping = false;
let finish;

function run(command, args, env = process.env) {
  const result = spawnSync(command, args, { cwd: root, env, stdio: 'inherit' });
  if (result.error || result.status !== 0) throw new Error(`${command} failed; check the diagnostics above.`);
}
function launch(command, args, env) {
  const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', detached: process.platform !== 'win32' });
  children.add(child);
  child.once('error', () => { console.error(`Cannot launch ${command}.`); finish(1); });
  child.once('exit', (code) => {
    children.delete(child);
    if (!stopping && !intentional.delete(child.pid)) { console.error('A project service exited; stopping the other services.'); stopping = true; finish(code || 1); }
  });
  return child;
}
async function terminate(child) {
  if (child.exitCode !== null || child.signalCode !== null || !child.pid) return;
  intentional.add(child.pid);
  function signal(name) { try { if (process.platform === 'win32') child.kill(name); else process.kill(-child.pid, name); } catch (error) { if (error.code !== 'ESRCH') throw error; } }
  signal('SIGTERM');
  let timeout;
  await Promise.race([new Promise((resolve) => child.once('exit', resolve)), new Promise((resolve) => { timeout = setTimeout(resolve, 5000); })]);
  clearTimeout(timeout);
  if (child.exitCode === null && child.signalCode === null) signal('SIGKILL');
}
async function ready(address, child) {
  const parsed = new URL(endpoint(address));
  const deadline = Date.now() + 30000;
  while (!stopping && child.exitCode === null && child.signalCode === null && Date.now() < deadline) {
    try { if ((await fetch(new URL('/health/ready', parsed), { signal: AbortSignal.timeout(1000) })).ok) return; } catch {}
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error('A project backend did not become ready; check its configuration and diagnostics.');
}
async function main() {
  const command = process.argv[2] ?? 'dev';
  if (!['dev', 'preview'].includes(command)) throw new Error('Supported application commands: dev, preview.');
  const env = loadEnvironment(root);
  const configs = Object.fromEntries(['iam', 'console'].map((module) => [module, path.join(root, `src/apps/${module}/config.yaml`)]));
  for (const file of Object.values(configs)) if (!existsSync(file)) throw new Error('Missing module config.yaml; restore the module configuration before starting.');
  const web = webListener(env);
  if (command === 'dev') run('pnpm', ['install', '--frozen-lockfile', '--ignore-scripts'], { ...webEnvironment(env), HUSKY: '0' });
  run('go', ['run', './src/apps/iam/cmd', 'migrate', '--config', configs.iam], moduleEnvironment(env, 'iam', true));
  const completion = new Promise((resolve) => { finish = resolve; });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { stopping = true; finish(0); });
  const services = {};
  const buildDirectory = mkdtempSync(path.join(tmpdir(), 'spx-dev-'));
  try {
    for (const module of ['iam', 'console']) {
      const output = path.join(buildDirectory, module);
      mkdirSync(path.dirname(output), { recursive: true });
      function build() { run('go', ['build', '-o', `${output}.next`, `./src/apps/${module}/cmd`]); }
      build(); renameSync(`${output}.next`, output);
      const args = [...(module === 'iam' ? ['serve'] : []), '--config', configs[module]];
      const moduleEnv = moduleEnvironment(env, module);
      if (module === 'console') moduleEnv.SPX_CONSOLE_IAM_BASE_URL = consoleEndpoint(env, services.iam);
      const inspected = spawnSync(output, ['--config', configs[module], '--print-endpoints'], { cwd: root, env: moduleEnv, encoding: 'utf8' });
      if (inspected.status !== 0) throw new Error(`Invalid ${module} config.yaml; check its fields and environment variables.`);
      services[module] = JSON.parse(inspected.stdout);
      let current = launch(output, args, moduleEnv);
      await ready(services[module].public_addr, current);
      if (command === 'dev') {
        let timer; let rebuilding = false; let pending = false;
        async function rebuild() {
          if (stopping) return;
          if (rebuilding) { pending = true; return; }
          rebuilding = true;
          try { build(); await terminate(current); if (!stopping) { renameSync(`${output}.next`, output); current = launch(output, args, moduleEnv); } }
          catch (error) { console.error(`${module}: ${error.message} The existing process is retained when compilation fails.`); }
          finally { rebuilding = false; if (pending) { pending = false; void rebuild(); } }
        }
        for (const directory of [`src/apps/${module}`, 'src/packages/go']) {
          const watcher = watch(path.join(root, directory), { recursive: true }, (_, filename) => {
            if (!filename || !/\.(go|sql)$/.test(filename) || filename.endsWith('_test.go')) return;
            clearTimeout(timer); timer = setTimeout(() => void rebuild(), 200);
          });
          watchers.push(watcher);
        }
        watchers.push({ close: () => clearTimeout(timer) });
      }
    }
    launch('pnpm', ['--dir', 'src/apps/web', command === 'preview' ? 'start' : 'dev', '--hostname', web.host, '--port', String(web.port)], { ...webEnvironment(env), ...webEndpoints(env, services) });
    console.log('SecPlexus applications started. Dependencies are accessed through configured endpoints.');
    process.exitCode = await completion;
  } finally {
    stopping = true;
    for (const watcher of watchers) watcher.close();
    await Promise.all([...children].map(terminate));
    rmSync(buildDirectory, { recursive: true, force: true });
  }
}
try { await main(); } catch (error) { console.error(error.message); process.exitCode = 1; }
