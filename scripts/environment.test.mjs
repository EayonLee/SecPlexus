import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { loadEnvironment, webEnvironment, moduleEnvironment } from './environment.mjs';

function workspace(t) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'spx-config-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

for (const [script, args] of [['start.mjs', ['dev']], ['start.mjs', ['preview']], ['compose.mjs', []]]) {
  test(`${script} ${args.join(' ')} prepares missing configuration before launching services`, (t) => {
    const root = workspace(t);
    mkdirSync(path.join(root, 'scripts'));
    for (const file of ['environment.mjs', 'endpoints.mjs', script]) {
      copyFileSync(fileURLToPath(new URL(file, import.meta.url)), path.join(root, 'scripts', file));
    }
    writeFileSync(path.join(root, '.env.example'), 'SPX_WEB_BIND_HOST=127.0.0.1\n');
    const result = spawnSync(process.execPath, [path.join(root, 'scripts', script), ...args], {
      env: { PATH: '' }, encoding: 'utf8',
    });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /Created .env.*Review/);
    assert.equal(readFileSync(path.join(root, '.env'), 'utf8'), 'SPX_WEB_BIND_HOST=127.0.0.1\n');
    assert.doesNotMatch(result.stderr, /pnpm|Cannot run Docker/);
  });
}

test('environment overrides user configuration without rewriting the file', (t) => {
  const root = workspace(t);
  const content = 'SPX_WEB_BIND_HOST=0.0.0.0\nNEXT_PUBLIC_DEMO=false\n';
  writeFileSync(path.join(root, '.env'), content);
  const env = loadEnvironment(root, { SPX_WEB_BIND_HOST: 'localhost' });
  assert.equal(env.SPX_WEB_BIND_HOST, 'localhost');
  assert.equal(env.NEXT_PUBLIC_DEMO, 'false');
  assert.equal(readFileSync(path.join(root, '.env'), 'utf8'), content);
});

test('first startup creates private configuration and asks for review before proceeding', (t) => {
  const root = workspace(t);
  writeFileSync(path.join(root, '.env.example'), 'SPX_WEB_BIND_HOST=127.0.0.1\n');
  assert.throws(() => loadEnvironment(root, {}), /Created .env.*Review/);
  assert.equal(readFileSync(path.join(root, '.env'), 'utf8'), 'SPX_WEB_BIND_HOST=127.0.0.1\n');
  if (process.platform !== 'win32') assert.equal(statSync(path.join(root, '.env')).mode & 0o777, 0o600);
  assert.equal(loadEnvironment(root, {}).SPX_WEB_BIND_HOST, '127.0.0.1');
});

test('startup reports a missing template instead of creating empty configuration', (t) => {
  const root = workspace(t);
  assert.throws(() => loadEnvironment(root, {}), /Copy .env.example to .env/);
  assert.equal(existsSync(path.join(root, '.env')), false);
});

test('web receives public settings without backend credentials', () => {
  assert.deepEqual(webEnvironment({
    PATH: '/usr/bin', SPX_WEB_BIND_HOST: '127.0.0.1', NEXT_PUBLIC_DEMO: 'false',
    SPX_POSTGRES_URL: 'backend-only', SPX_POSTGRES_PASSWORD: 'test-only',
    SPX_REDIS_URL: 'backend-only',
  }), { PATH: '/usr/bin', SPX_WEB_BIND_HOST: '127.0.0.1', NEXT_PUBLIC_DEMO: 'false' });
});

test('only IAM receives authoritative administrator credentials and modules remain isolated', () => {
  const environment = { PATH: '/tools', SPX_POSTGRES_PASSWORD: 'database', SPX_REDIS_PASSWORD: 'redis', SPX_IAM_BOOTSTRAP_PASSWORD: 'initial', SPX_CONSOLE_IAM_TOKEN: 'console', NEXT_PUBLIC_DEMO: 'false' };
  assert.deepEqual(moduleEnvironment(environment, 'iam'), { PATH: '/tools', SPX_POSTGRES_PASSWORD: 'database', SPX_REDIS_PASSWORD: 'redis', SPX_IAM_BOOTSTRAP_PASSWORD: 'initial', SPX_CONSOLE_IAM_TOKEN: 'console' });
  assert.deepEqual(moduleEnvironment(environment, 'console'), { PATH: '/tools', SPX_CONSOLE_IAM_TOKEN: 'console' });
});
for (const command of ['dev', 'preview']) {
  test(`${command} rejects incomplete configuration before launching applications or infrastructure`, (t) => {
    const root = workspace(t); mkdirSync(path.join(root, 'scripts'));
    for (const file of ['environment.mjs', 'endpoints.mjs', 'start.mjs']) copyFileSync(fileURLToPath(new URL(file, import.meta.url)), path.join(root, 'scripts', file));
    writeFileSync(path.join(root, '.env'), 'SPX_WEB_BIND_HOST=127.0.0.1\n');
    const result = spawnSync(process.execPath, [path.join(root, 'scripts/start.mjs'), command], { env: { PATH: '' }, encoding: 'utf8' });
    assert.equal(result.status, 1); assert.match(result.stderr, /Missing module config.yaml/); assert.doesNotMatch(result.stderr, /Docker|pnpm failed/);
  });
}

test('root file paths become absolute without rewriting private configuration', (t) => {
  const root = workspace(t);
  const text = 'SPX_CONSOLE_LOG_FILE=logs/console.log\nSPX_IAM_LOG_FILE=logs/iam.log\n';
  writeFileSync(path.join(root, '.env'), text);
  const env = loadEnvironment(root, {});
  assert.equal(env.SPX_CONSOLE_LOG_FILE, path.join(root, 'logs/console.log'));
  assert.equal(env.SPX_IAM_LOG_FILE, path.join(root, 'logs/iam.log'));
  assert.equal(readFileSync(path.join(root, '.env'), 'utf8'), text);
});

test('Web does not receive the IAM trusted-origin configuration', () => {
  assert.deepEqual(webEnvironment({ SPX_WEB_ORIGIN: 'https://sec.example.com', SPX_CONSOLE_IAM_TOKEN: 'server-only' }), {});
});

test('module connection overrides reach IAM only and stay outside Web and Console', () => {
  const environment = {SPX_IAM_POSTGRES_HOST:'isolated-db',SPX_IAM_POSTGRES_PASSWORD:'test-only',SPX_IAM_REDIS_URL:'isolated-cache',SPX_POSTGRES_HOST:'shared-db',SPX_WEB_BIND_HOST:'127.0.0.1'};
  assert.deepEqual(moduleEnvironment(environment,'iam'),{SPX_IAM_POSTGRES_HOST:'isolated-db',SPX_IAM_POSTGRES_PASSWORD:'test-only',SPX_IAM_REDIS_URL:'isolated-cache',SPX_POSTGRES_HOST:'shared-db'});
  assert.deepEqual(moduleEnvironment(environment,'console'),{});
  assert.deepEqual(webEnvironment(environment),{SPX_WEB_BIND_HOST:'127.0.0.1'});
});

test('one shared service location reaches Web and Console without forwarding IAM secrets',()=>{
 const environment={SPX_IAM_HOST:'10.0.0.12',SPX_IAM_PORT:'18081',SPX_CONSOLE_HOST:'console.corp',SPX_CONSOLE_PORT:'18080',SPX_WEB_PORT:'15173',SPX_CONSOLE_IAM_TOKEN:'service-only',SPX_IAM_BOOTSTRAP_PASSWORD:'iam-only'};
 assert.deepEqual(webEnvironment(environment),{SPX_IAM_HOST:'10.0.0.12',SPX_IAM_PORT:'18081',SPX_CONSOLE_HOST:'console.corp',SPX_CONSOLE_PORT:'18080',SPX_WEB_PORT:'15173'});
 assert.deepEqual(moduleEnvironment(environment,'console'),{SPX_IAM_HOST:'10.0.0.12',SPX_IAM_PORT:'18081',SPX_CONSOLE_HOST:'console.corp',SPX_CONSOLE_PORT:'18080',SPX_CONSOLE_IAM_TOKEN:'service-only'});
});
