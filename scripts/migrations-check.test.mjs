import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { checkMigrationBoundaries, foreignKeyTargets, moduleSchemas } from './migrations-check.mjs';

function workspace() {
  const root = mkdtempSync(join(tmpdir(), 'spx-mig-'));
  return { root, cleanup: () => rmSync(root, { recursive: true, force: true }) };
}

function writeModule(root, app, files, database = '') {
  const dir = join(root, app, 'migrations', database);
  mkdirSync(dir, { recursive: true });
  for (const [name, sql] of Object.entries(files)) writeFileSync(join(dir, name), sql);
}

test('the CLI checks nested PostgreSQL migrations and excludes ClickHouse migrations', () => {
  const { root, cleanup } = workspace();
  try {
    const apps = join(root, 'src', 'apps');
    writeModule(apps, 'ingest', {
      '00001_init.sql': 'CREATE TABLE ingest.inputs (id uuid PRIMARY KEY);',
    }, 'postgresql');
    writeModule(apps, 'ingest', {
      '00001_init.sql': 'CREATE TABLE spx.events (id UUID);',
    }, 'clickhouse');
    const cli = fileURLToPath(new URL('./migrations-check.mjs', import.meta.url));
    const check = () => spawnSync(process.execPath, [cli], { cwd: root, encoding: 'utf8' });
    assert.equal(check().status, 0);

    writeModule(apps, 'ingest', {
      '00002_owner.sql': 'ALTER TABLE ingest.inputs ADD COLUMN owner uuid REFERENCES iam.users(id);',
    }, 'postgresql');
    const result = check();
    assert.equal(result.status, 1);
    assert.match(result.stderr, /ingest\/migrations\/postgresql\/00002_owner\.sql REFERENCES iam\.users/);
  } finally {
    cleanup();
  }
});

test('same-schema references pass and module schema is inferred from CREATE TABLE', () => {
  const { root, cleanup } = workspace();
  try {
    writeModule(root, 'iam', {
      '00001_a.sql': 'CREATE TABLE iam.users (id uuid PRIMARY KEY);',
      '00002_b.sql': 'CREATE TABLE iam.sessions(user_id uuid REFERENCES iam.users(id));',
    });
    const modules = moduleSchemas(root);
    assert.equal(modules.get('iam').schema, 'iam');
    assert.deepEqual(checkMigrationBoundaries(root), []);
  } finally {
    cleanup();
  }
});

test('cross-schema foreign keys are reported with module, file and target', () => {
  const { root, cleanup } = workspace();
  try {
    writeModule(root, 'ingestor', {
      '00001_a.sql': 'CREATE SCHEMA ingestor; CREATE TABLE ingestor.connectors(id uuid PRIMARY KEY);',
      '00002_b.sql': 'ALTER TABLE ingestor.connectors ADD COLUMN owner uuid REFERENCES iam.users(id);',
    });
    assert.deepEqual(checkMigrationBoundaries(root), [
      { module: 'ingestor', file: '00002_b.sql', target: 'iam.users' },
    ]);
  } finally {
    cleanup();
  }
});

test('a module touching two schemas is rejected outright', () => {
  const { root, cleanup } = workspace();
  try {
    writeModule(root, 'mixed', {
      '00001_a.sql': 'CREATE TABLE a.t1(id int);',
      '00002_b.sql': 'CREATE TABLE b.t2(id int);',
    });
    assert.throws(() => checkMigrationBoundaries(root), /multiple schemas/);
  } finally {
    cleanup();
  }
});

test('unqualified references are ignored, ONLY form is extracted', () => {
  assert.deepEqual(foreignKeyTargets('REFERENCES users(id)'), []);
  assert.deepEqual(foreignKeyTargets('ALTER TABLE t ADD FOREIGN KEY (u) REFERENCES ONLY iam.users(id)'), ['iam.users']);
  assert.deepEqual(foreignKeyTargets('REFERENCES ingestor.connectors(id)'), ['ingestor.connectors']);
});

test('directories without migrations are skipped', () => {
  const { root, cleanup } = workspace();
  try {
    mkdirSync(join(root, 'web'), { recursive: true });
    assert.equal(moduleSchemas(root).size, 0);
    assert.deepEqual(checkMigrationBoundaries(root), []);
  } finally {
    cleanup();
  }
});
