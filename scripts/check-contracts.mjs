import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
const specifications = ['iam','console'].map((module) => JSON.parse(readFileSync(`src/contracts/${module}/openapi.json`, 'utf8')));
const catalog = JSON.parse(readFileSync('src/contracts/iam/permissions.json', 'utf8'));
const permissions = new Set();
for (const permission of catalog) {
  assert(!permissions.has(permission.key), `Duplicate permission ${permission.key}`); permissions.add(permission.key);
  assert(['platform', 'organization'].includes(permission.scope));
  assert(!(permission.scope === 'platform' && permission.delegable));
}
const declared = new Set();
for (const specification of specifications) for (const [pathname, methods] of Object.entries(specification.paths)) {
  assert(/^\/(api|internal)\/v1\/(iam|console)\//.test(pathname), `Invalid API prefix ${pathname}`);
  for (const [method, operation] of Object.entries(methods)) {
    declared.add(`${method.toUpperCase()} ${pathname}`);
    for (const action of [operation['x-spx-action'] || []].flat()) assert(permissions.has(action), `Unknown action ${action}`);
  }
}
const result = spawnSync('go', ['test', './src/apps/iam/internal/router', './src/apps/console/internal/router'], { stdio: 'inherit' });
if (result.error || result.status !== 0) process.exit(result.status || 1);
const codes = JSON.parse(readFileSync('src/contracts/common/error-codes.json', 'utf8'));
assert.equal(new Set(codes.map((entry) => entry.code)).size, codes.length, 'Duplicate result codes');
for (const entry of codes) {
  assert(entry.code === 'ok' ? entry.status === 200 : entry.status >= 400 && entry.status <= 599);
  assert(entry.message && entry.module);
}
console.log(`API contract checked: ${declared.size} mounted routes, ${permissions.size} permissions, ${codes.length} result codes.`);
