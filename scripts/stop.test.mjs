import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const scripts = fileURLToPath(new URL('.', import.meta.url));

function workspace(t, files = ['stop.mjs', 'runtime.mjs']) {
  const root = realpathSync(mkdtempSync(path.join(os.tmpdir(), 'spx-stop-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'scripts'), { recursive: true });
  for (const file of files) copyFileSync(path.join(scripts, file), path.join(root, 'scripts', file));
  return root;
}

function stop(root, environment = process.env) {
  return execFileSync(process.execPath, [path.join(root, 'scripts/stop.mjs')], {
    env: environment, encoding: 'utf8',
  });
}

for (const docker of ['missing', 'unavailable']) {
  test(`stop works for application-only development when Docker is ${docker}`, (t) => {
    const root = workspace(t);
    for (const directory of ['deploy/compose', 'tools']) mkdirSync(path.join(root, directory), { recursive: true });
    writeFileSync(path.join(root, 'deploy/compose/compose.yaml'), 'services: {}\n');
    for (const command of ['ps', 'lsof']) {
      writeFileSync(path.join(root, 'tools', command), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
    }
    writeFileSync(path.join(root, 'tools/docker'), '#!/bin/sh\nexit 1\n', { mode: 0o700 });
    const output = stop(root, { ...process.env, PATH: path.join(root, 'tools') });
    assert.match(output, /container status is unknown/i);
    assert.doesNotMatch(output, /No SecPlexus services are running/);
  });
}

test('stop terminates a registered service tree', { timeout: 20000 }, async (t) => {
  const root = workspace(t);
  mkdirSync(path.join(root, 'src/apps/iam'), { recursive: true });
  const listenerFile = path.join(root, 'src/apps/iam/listener.mjs');
  writeFileSync(listenerFile, "import net from 'node:net'; net.createServer().listen(0,'127.0.0.1',()=>console.log('ready'));\n");
  const child = spawn(process.execPath, [listenerFile], {
    cwd: root, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'ignore'],
  });
  t.after(() => { try { child.kill('SIGKILL'); } catch {} });
  await once(child.stdout, 'data');
  mkdirSync(path.join(root, '.spx'), { recursive: true });
  writeFileSync(path.join(root, '.spx/run.json'), JSON.stringify({
    version: 1,
    processes: [{ pid: child.pid, module: 'iam', ...(process.platform === 'win32' ? { image: 'node.exe' } : {}) }],
  }));
  const exited = once(child, 'exit');
  const output = stop(root);
  assert.match(output, /Stopping 1 project service process/);
  await exited;
  assert.equal(existsSync(path.join(root, '.spx/run.json')), false);
});

test('stop clears a stale registry without killing unrelated processes', (t) => {
  const root = workspace(t);
  mkdirSync(path.join(root, '.spx'), { recursive: true });
  writeFileSync(path.join(root, '.spx/run.json'), JSON.stringify({
    version: 1,
    processes: [{ pid: 99999999, module: 'iam' }],
  }));
  const output = stop(root);
  assert.match(output, /No SecPlexus services are running/);
  assert.equal(existsSync(path.join(root, '.spx/run.json')), false);
});

test('stop terminates an owned listener and retains an unrelated process', { skip: process.platform === 'win32' }, async (t) => {
  const root = workspace(t);
  for (const directory of ['src/apps/iam', 'tools']) mkdirSync(path.join(root, directory), { recursive: true });
  const listener = path.join(root, 'src/apps/iam/listener.mjs');
  writeFileSync(listener, "import net from 'node:net'; net.createServer().listen(0,'127.0.0.1',()=>console.log('ready'));\n");
  const child = spawn(process.execPath, [listener], { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] });
  const unrelated = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { cwd: os.tmpdir(), stdio: 'ignore' });
  t.after(() => { child.kill(); unrelated.kill(); });
  await once(child.stdout, 'data');
  // Only the port inventory is controlled; ownership comes from the real process state.
  writeFileSync(path.join(root, 'tools/lsof'), `#!/bin/sh
case "$*" in
  *-iTCP*) printf 'p${child.pid}\np${unrelated.pid}\n' ;;
  *) printf 'p${child.pid}\nn${root}\np${unrelated.pid}\nn${os.tmpdir()}\n' ;;
esac
`, { mode: 0o700 });
  const output = stop(root, {
    ...process.env,
    PATH: `${path.join(root, 'tools')}${path.delimiter}${process.env.PATH}`,
  });
  assert.match(output, /Stopping 1 project service process/);
  assert.doesNotThrow(() => process.kill(unrelated.pid, 0));
  await once(child, 'exit');
});
