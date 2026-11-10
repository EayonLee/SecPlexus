import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { terminateProcessTree } from './processes.mjs';

function running(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function waitFor(predicate, message) {
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  assert.fail(message);
}

test('terminating a dev process leaves no descendants', { timeout: 15000 }, async (t) => {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'spx-process-tree-'));
  const pidFile = path.join(directory, 'grandchild.pid');
  const wrapperFile = path.join(directory, 'wrapper.cjs');
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  writeFileSync(wrapperFile, `
    const { spawn } = require('node:child_process');
    const { writeFileSync } = require('node:fs');
    const child = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], { stdio: 'ignore' });
    writeFileSync(${JSON.stringify(pidFile)}, String(child.pid));
    setInterval(() => {}, 1000);
  `);

  // The wrapper is started without a shell on both platforms: taskkill /T walks the
  // Windows process tree, and detached: true makes the POSIX child a process-group
  // leader. Passing environment paths through cmd.exe or /bin/sh would only add a
  // command interpreter to the tree (and an injection sink) without testing anything.
  const command = { file: process.execPath, args: [wrapperFile] };
  const wrapper = spawn(command.file, command.args, {
    detached: process.platform !== 'win32',
    stdio: 'ignore',
  });
  t.after(() => {
    if (running(wrapper.pid)) process.kill(wrapper.pid, 'SIGKILL');
  });

  await waitFor(() => existsSync(pidFile), 'grandchild did not record its pid');
  const grandchild = Number(readFileSync(pidFile, 'utf8'));
  t.after(() => {
    if (running(grandchild)) process.kill(grandchild, 'SIGKILL');
  });
  assert.equal(running(grandchild), true);

  await terminateProcessTree(wrapper);

  await waitFor(() => !running(grandchild), 'termination left the grandchild running');
});
