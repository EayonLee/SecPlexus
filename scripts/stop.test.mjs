import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { copyFileSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

for (const docker of ['missing', 'unavailable']) {
  test(`stop works for application-only development when Docker is ${docker}`, (t) => {
    const root = mkdtempSync(path.join(os.tmpdir(), 'spx-stop-'));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    for (const directory of ['scripts', 'deploy/compose', 'tools']) {
      mkdirSync(path.join(root, directory), { recursive: true });
    }
    copyFileSync(fileURLToPath(new URL('./stop.mjs', import.meta.url)), path.join(root, 'scripts/stop.mjs'));
    writeFileSync(path.join(root, 'deploy/compose/compose.yaml'), 'services: {}\n');
    for (const command of ['ps', 'lsof']) {
      writeFileSync(path.join(root, 'tools', command), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
    }
    if (docker === 'unavailable') {
      writeFileSync(path.join(root, 'tools/docker'), '#!/bin/sh\nexit 1\n', { mode: 0o700 });
    }
    const output = execFileSync(process.execPath, [path.join(root, 'scripts/stop.mjs')], {
      env: { ...process.env, PATH: path.join(root, 'tools') }, encoding: 'utf8',
    });
    assert.match(output, /container status is unknown/i);
    assert.doesNotMatch(output, /No SecPlexus services are running/);
  });
}


test('stop terminates an owned listener and retains an unrelated process', { skip: process.platform === 'win32' }, async (t) => {
  const root = realpathSync(mkdtempSync(path.join(os.tmpdir(), 'spx-stop-owned-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const directory of ['scripts', 'src/apps/iam', 'tools']) mkdirSync(path.join(root, directory), { recursive: true });
  copyFileSync(fileURLToPath(new URL('./stop.mjs', import.meta.url)), path.join(root, 'scripts/stop.mjs'));
  const listener = path.join(root, 'src/apps/iam/listener.mjs');
  writeFileSync(listener, "import net from 'node:net'; net.createServer().listen(0,'127.0.0.1',()=>console.log('ready'));\n");
  const child = spawn(process.execPath, [listener], { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] });
  const unrelated = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { cwd: os.tmpdir(), stdio: 'ignore' });
  t.after(() => { child.kill(); unrelated.kill(); });
  await once(child.stdout, 'data');
  // Use real processes and cwd ownership; only the port inventory is controlled.
  writeFileSync(path.join(root, 'tools/lsof'), `#!/bin/sh
case "$*" in
  *-iTCP*) printf 'p${child.pid}\np${unrelated.pid}\n' ;;
  *) printf 'p${child.pid}\nn${root}\np${unrelated.pid}\nn${os.tmpdir()}\n' ;;
esac
`, { mode: 0o700 });
  const output = execFileSync(process.execPath, [path.join(root, 'scripts/stop.mjs')], {
    env: { ...process.env, PATH: `${path.join(root, 'tools')}${path.delimiter}${process.env.PATH}` }, encoding: 'utf8',
  });
  assert.match(output, /Stopping 1 project service process/);
  assert.doesNotThrow(() => process.kill(unrelated.pid, 0));
  await once(child, 'exit');
});
