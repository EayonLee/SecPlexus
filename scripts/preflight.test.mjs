import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
test('all selected modules are checked before migration or runtime side effects', { timeout: 120000 }, () => {
  const directory = mkdtempSync(path.join(tmpdir(), 'spx-preflight-'));
  try {
    for (const entry of ['go.mod', 'go.sum', 'scripts', 'src/apps/iam', 'src/apps/console', 'src/packages/go']) {
      mkdirSync(path.dirname(path.join(directory, entry)), { recursive: true });
      cpSync(path.join(root, entry), path.join(directory, entry), { recursive: true });
    }
    writeFileSync(path.join(directory, '.env'), '', { mode: 0o600 });
    const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('SPX_')));
    Object.assign(env, {
      SPX_WEB_PORT: '15173', SPX_IAM_CSRF_KEY: 'c'.repeat(32), SPX_CONSOLE_IAM_TOKEN: 't'.repeat(32),
      SPX_POSTGRES_HOST: '127.0.0.1', SPX_POSTGRES_PORT: '1',
      SPX_REDIS_HOST: '127.0.0.1', SPX_REDIS_PORT: '1', SPX_IAM_PORT: '18081', SPX_CONSOLE_BIND_PORT: '0',
      // IAM is valid; Console's listener is deliberately invalid.
      SPX_IAM_BIND_PORT: '18081',
    });
    const result = spawnSync(process.execPath, ['scripts/start.mjs', 'preview', 'iam', 'console'], {
      cwd: directory, env, encoding: 'utf8', timeout: 110000,
    });
    assert.equal(result.status, 1);
    assert.match(result.stdout + result.stderr, /app.port/);
    assert.doesNotMatch(result.stdout + result.stderr, /migrations and initialization completed|connection refused/);
    assert.equal(existsSync(path.join(directory, 'logs')), false);
    env.SPX_CONSOLE_BIND_PORT = '18080';
    for (const modules of [['iam', 'console'], ['iam'], ['console']]) {
      const check = spawnSync(process.execPath, ['scripts/start.mjs', 'check-config', ...modules], {
        cwd: directory, env, encoding: 'utf8', timeout: 110000,
      });
      assert.equal(check.status, 0, check.stderr);
      assert.match(check.stdout, /configuration valid \(offline\)/);
      assert.equal(existsSync(path.join(directory, 'logs')), false);
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
