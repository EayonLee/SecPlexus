import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, renameSync, rmSync, watch } from 'node:fs';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { loadEnvironment, moduleEnvironment, webEnvironment } from './environment.mjs';
import { endpoint, serviceConnection, webEndpoints, webListener, webOrigin } from './endpoints.mjs';

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
async function ready(service, child) {
  const target = new URL('/health/ready', endpoint(service.public_addr));
  const deadline = Date.now() + 30000;
  while (!stopping && child.exitCode === null && child.signalCode === null && Date.now() < deadline) {
    try { if ((await fetch(target, {signal: AbortSignal.timeout(1000)})).ok) return; } catch {}
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error('A project backend did not become ready; check its configuration and diagnostics.');
}
async function main() {
  const command = process.argv[2] ?? 'dev';
  if (!['dev', 'preview', 'check-config'].includes(command)) throw new Error('Supported application commands: dev, preview, check-config.');
  const selected = process.argv.slice(3);
  const modules = selected.length ? selected : ['iam','console','web'];
  if (new Set(modules).size !== modules.length || modules.some(module => !['iam','console','web'].includes(module))) throw new Error('Select web, iam or console once each.');
  const env = loadEnvironment(root);
  const configs = Object.fromEntries(['iam', 'console'].map((module) => [module, path.join(root, `src/apps/${module}/config.yaml`)]));
  for (const module of modules.filter(module => module !== 'web')) if (!existsSync(configs[module])) throw new Error(`Missing module config.yaml (src/apps/${module}/config.yaml); restore the module configuration before starting.`);
  if (modules.includes('iam')) env.SPX_WEB_ORIGIN = webOrigin(env);
  if (modules.includes('web')) webListener(env);
  if (command === 'dev' && modules.includes('web')) run('pnpm', ['install', '--frozen-lockfile', '--ignore-scripts'], { ...webEnvironment(env), HUSKY: '0' });
  const completion = new Promise((resolve) => { finish = resolve; });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { stopping = true; finish(0); });
  const services = {};
  const buildDirectory = mkdtempSync(path.join(tmpdir(), 'spx-dev-'));
  try {
    const prepared = {};
    for (const module of ['iam', 'console'].filter(module => modules.includes(module))) {
      const output = path.join(buildDirectory, module);
      run('go', ['build', '-o', output, `./src/apps/${module}/cmd`]);
      const moduleEnv = moduleEnvironment(module === 'console' && services.iam ? {...env,...serviceConnection(env,'iam',services.iam)} : env, module);
      const inspected = spawnSync(output, ['check-config', '--config', configs[module], '--print-endpoints'], { cwd: root, env: moduleEnv, encoding: 'utf8' });
      if (inspected.error || inspected.status !== 0) {
        if (inspected.stderr) process.stderr.write(`${module}: ${inspected.stderr}`);
        throw new Error(`${module} configuration check failed; no migrations or services were started.`);
      }
      services[module] = JSON.parse(inspected.stdout);
      prepared[module] = { output, moduleEnv };
    }
    const webEnv = modules.includes('web') ? { ...webEnvironment(env), ...webEndpoints(env, services) } : undefined;
    if (webEnv) run(process.execPath, ['src/apps/web/scripts/serve.mjs', 'check-config', ...(command === 'dev' ? ['--development'] : [])], webEnv);
    if (command === 'check-config') { console.log(`SecPlexus ${modules.join(', ')} configuration valid (offline).`); return; }
    if (prepared.iam) run(prepared.iam.output, ['migrate', '--config', configs.iam], prepared.iam.moduleEnv);
    for (const module of ['iam', 'console'].filter(module => modules.includes(module))) {
      const { output, moduleEnv } = prepared[module];
      const args = ['serve', '--config', configs[module]];
      function build() { run('go', ['build', '-o', `${output}.next`, `./src/apps/${module}/cmd`]); }
      let current = launch(output, args, moduleEnv);
      await ready(services[module], current);
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
    if (modules.includes('web')) launch('pnpm', ['--dir', 'src/apps/web', command === 'preview' ? 'start' : 'dev'], webEnv);
    console.log(`SecPlexus ${modules.join(', ')} started. Dependencies use configured endpoints.`);
    process.exitCode = await completion;
  } finally {
    stopping = true;
    for (const watcher of watchers) watcher.close();
    await Promise.all([...children].map(terminate));
    rmSync(buildDirectory, { recursive: true, force: true });
  }
}
try { await main(); } catch (error) { console.error(error.message); process.exitCode = 1; }
