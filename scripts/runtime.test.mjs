import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { clearRuntime, processAlive, readRuntime, runtimeFile, writeRuntime } from './runtime.mjs';

function workspace(t) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'spx-runtime-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

test('runtime registry round-trips the service roots and replaces stale entries', (t) => {
  const root = workspace(t);
  writeRuntime(root, [{ pid: 123, module: 'iam', image: 'iam.exe' }]);
  assert.deepEqual(readRuntime(root), [{ pid: 123, module: 'iam', image: 'iam.exe' }]);
  writeRuntime(root, [{ pid: 456, module: 'web', image: 'cmd.exe' }]);
  assert.deepEqual(readRuntime(root), [{ pid: 456, module: 'web', image: 'cmd.exe' }]);
  assert.equal(readFileSync(runtimeFile(root), 'utf8').endsWith('\n'), true);
  clearRuntime(root);
  assert.equal(existsSync(runtimeFile(root)), false);
});

test('runtime registry ignores malformed state and invalid entries', (t) => {
  const root = workspace(t);
  mkdirSync(path.dirname(runtimeFile(root)), { recursive: true });
  writeFileSync(runtimeFile(root), '{not json');
  assert.deepEqual(readRuntime(root), []);
  writeRuntime(root, [{ pid: 1, module: 'iam' }, { pid: -1, module: 'console' }, { nope: true }]);
  assert.deepEqual(readRuntime(root), [{ pid: 1, module: 'iam' }]);
});

test('processAlive distinguishes the current process from a missing pid', () => {
  assert.equal(processAlive(process.pid), true);
  assert.equal(processAlive(99999999), false);
});
