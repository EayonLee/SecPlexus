import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, renameSync, rmSync, watch } from 'node:fs';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { loadEnvironment, moduleEnvironment, webEnvironment } from './environment.mjs';
import { endpoint, serviceConnection, webEndpoints, webListener, webOrigin } from './endpoints.mjs';
import { terminateProcessTree } from './processes.mjs';
import { assertStackAvailable, clearRuntime, writeRuntime } from './runtime.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
// Windows cannot launch a PE image without the .exe extension, so builds keep it there.
const executableSuffix = process.platform === 'win32' ? '.exe' : '';
const children = new Set();
const intentional = new Set();
const runtime = new Map();
const watchers = [];
let stopping = false;
let finish;

// pnpm ships as a .cmd shim on Windows, which only a shell can launch.
const pnpmShell = process.platform === 'win32' ? { shell: true } : {};
function processImage(command) {
  if (process.platform !== 'win32') return undefined;
  return command === 'pnpm' ? 'cmd.exe' : path.basename(command);
}
function syncRuntime() {
  if (stopping && runtime.size === 0) return;
  try {
    writeRuntime(root, [...runtime.values()].sort((left, right) => left.pid - right.pid));
  } catch (error) {
    console.error(`Cannot update .spx/run.json: ${error.message}`);
  }
}
// Windows 上刚退出的子进程可能短暂持有可执行文件句柄；清理失败不影响命令结果，
// 编译目录位于系统临时目录，不属于项目状态。
function removeBuildDirectory(directory) {
  try { rmSync(directory, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); } catch {}
}
function run(command, args, env = process.env) {
  const result = spawnSync(command, args, { cwd: root, env, stdio: 'inherit', ...(command === 'pnpm' ? pnpmShell : {}) });
  if (result.error || result.status !== 0) throw new Error(`${command} failed; check the diagnostics above.`);
}
function launch(command, args, env, module) {
  const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', detached: process.platform !== 'win32', ...(command === 'pnpm' ? pnpmShell : {}) });
  children.add(child);
  if (child.pid) {
    const image = processImage(command);
    runtime.set(child.pid, { pid: child.pid, module, ...(image ? { image } : {}) });
    syncRuntime();
  }
  child.once('error', () => { console.error(`Cannot launch ${command}.`); finish(1); });
  child.once('exit', (code) => {
    children.delete(child);
    runtime.delete(child.pid);
    syncRuntime();
    if (!stopping && !intentional.delete(child.pid)) { console.error('A project service exited; stopping the other services.'); stopping = true; finish(code || 1); }
  });
  return child;
}
async function terminate(child) {
  if (child.exitCode !== null || child.signalCode !== null || !child.pid) return;
  intentional.add(child.pid);
  await terminateProcessTree(child);
}
async function ready(service, child) {
  const target = new URL('/health/ready', endpoint(service.public_addr, service.scheme));
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
  const supported = ['iam', 'console', 'ingest', 'web'];
  const modules = selected.length ? selected : supported;
  if (new Set(modules).size !== modules.length || modules.some(module => !supported.includes(module))) throw new Error('Select web, iam, ingest or console once each.');
  const env = loadEnvironment(root);
  const configs = Object.fromEntries(['iam', 'console', 'ingest'].map((module) => [module, path.join(root, `src/apps/${module}/config.yaml`)]));
  for (const module of modules.filter(module => module !== 'web')) if (!existsSync(configs[module])) throw new Error(`Missing module config.yaml (src/apps/${module}/config.yaml); restore the module configuration before starting.`);
  if (modules.includes('iam')) env.SPX_WEB_ORIGIN = webOrigin(env);
  if (modules.includes('web')) webListener(env);
  if (command !== 'check-config') {
    assertStackAvailable(root, modules.includes('web'));
    clearRuntime(root);
  }
  if (command === 'dev' && modules.includes('web')) run('pnpm', ['install', '--frozen-lockfile', '--ignore-scripts'], { ...webEnvironment(env), HUSKY: '0' });
  const completion = new Promise((resolve) => { finish = resolve; });
  for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { stopping = true; finish(0); });
  const services = {};
  const buildDirectory = mkdtempSync(path.join(tmpdir(), 'spx-dev-'));
  try {
    const prepared = {};
    for (const module of ['iam', 'console', 'ingest'].filter(module => modules.includes(module))) {
      const output = path.join(buildDirectory, module + executableSuffix);
      run('go', ['build', '-o', output, `./src/apps/${module}/cmd`]);
      const moduleEnv = moduleEnvironment((module === 'console' || module === 'ingest') && services.iam ? {...env,...serviceConnection(env,'iam',services.iam)} : env, module);
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
    if (prepared.iam) {
      run(prepared.iam.output, ['migrate', '--config', configs.iam], prepared.iam.moduleEnv);
      run(prepared.iam.output, ['initialize-administrator', '--config', configs.iam], prepared.iam.moduleEnv);
    }
    // ingest 的表归它自己：不迁移就启动会在首个管理请求上失败。
    if (prepared.ingest) run(prepared.ingest.output, ['migrate', '--config', configs.ingest], prepared.ingest.moduleEnv);
    for (const module of ['iam', 'console', 'ingest'].filter(module => modules.includes(module))) {
      const { output, moduleEnv } = prepared[module];
      const args = ['serve', '--config', configs[module]];
      function build() { run('go', ['build', '-o', `${output}.next`, `./src/apps/${module}/cmd`]); }
      let current = launch(output, args, moduleEnv, module);
      await ready(services[module], current);
      if (command === 'dev') {
        let timer; let rebuilding = false; let pending = false;
        async function rebuild() {
          if (stopping) return;
          if (rebuilding) { pending = true; return; }
          rebuilding = true;
          try { build(); await terminate(current); if (!stopping) { renameSync(`${output}.next`, output); current = launch(output, args, moduleEnv, module); } }
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
    if (modules.includes('web')) launch('pnpm', ['--dir', 'src/apps/web', command === 'preview' ? 'start' : 'dev'], webEnv, 'web');
    console.log(`SecPlexus ${modules.join(', ')} started. Dependencies use configured endpoints.`);
    process.exitCode = await completion;
  } finally {
    stopping = true;
    for (const watcher of watchers) watcher.close();
    await Promise.all([...children].map(terminate));
    if (command !== 'check-config') clearRuntime(root);
    removeBuildDirectory(buildDirectory);
  }
}
try { await main(); } catch (error) { console.error(error.message); process.exitCode = 1; }
