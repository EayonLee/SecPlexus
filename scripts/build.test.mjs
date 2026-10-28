import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

function buildWorkspace(t, configuration, overrides = {}) {
  const root = mkdtempSync(path.join(tmpdir(), 'spx-build-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'scripts'));
  mkdirSync(path.join(root, 'tools'));
  for (const file of ['build.mjs', 'environment.mjs']) {
    copyFileSync(fileURLToPath(new URL(file, import.meta.url)), path.join(root, 'scripts', file));
  }
  if (configuration !== undefined) writeFileSync(path.join(root, '.env'), configuration);
  writeFileSync(path.join(root, 'tools/go'), `#!${process.execPath}\n`, { mode: 0o755 });
  writeFileSync(path.join(root, 'tools/pnpm'), `#!${process.execPath}
require('node:fs').writeFileSync('web-build.json', JSON.stringify({
  publicURL: process.env.NEXT_PUBLIC_APP_URL,
  iamOrigin: process.env.SPX_WEB_IAM_PROXY_TARGET,
  databasePassword: process.env.SPX_POSTGRES_PASSWORD,
  internalToken: process.env.SPX_CONSOLE_IAM_TOKEN,
}));
`, { mode: 0o755 });
  const result = spawnSync(process.execPath, [path.join(root, 'scripts/build.mjs')], {
    env: { PATH: path.join(root, 'tools'), ...overrides }, encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.stderr);
  return { root, web: JSON.parse(readFileSync(path.join(root, 'web-build.json'), 'utf8')) };
}

test('root build passes public configuration to Web and excludes backend credentials', (t) => {
  const configuration = 'SPX_PUBLIC_URL=https://spx.example\nSPX_WEB_IAM_PROXY_TARGET=https://iam.example\nSPX_POSTGRES_PASSWORD=database-only\nSPX_CONSOLE_IAM_TOKEN=internal-only\n';
  const { root, web } = buildWorkspace(t, configuration);
  assert.deepEqual(web, { publicURL: 'https://spx.example', iamOrigin: 'https://iam.example' });
  assert.equal(readFileSync(path.join(root, '.env'), 'utf8'), configuration);
});

test('root build honors an explicit public metadata override', (t) => {
  const { web } = buildWorkspace(t, 'SPX_PUBLIC_URL=https://spx.example\nNEXT_PUBLIC_APP_URL=https://file.example\n', {
    NEXT_PUBLIC_APP_URL: 'https://process.example',
  });
  assert.equal(web.publicURL, 'https://process.example');
});

test('root build works without creating development configuration', (t) => {
  const { root } = buildWorkspace(t);
  assert.equal(existsSync(path.join(root, '.env')), false);
});
