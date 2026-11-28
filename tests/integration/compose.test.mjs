import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));

function composeConfig(t, settings, services = []) {
  const directory = mkdtempSync(path.join(tmpdir(), 'spx-compose-config-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const envFile = path.join(directory, 'settings.env');
  writeFileSync(envFile, '', { mode: 0o600 });
  const system = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('SPX_')));
  const result = spawnSync('docker', [
    'compose', '--project-directory', root, '--env-file', envFile, '-f', 'deploy/compose/compose.yaml',
    'config', '--format', 'json', ...services,
  ], {
    cwd: root, env: { ...system, ...settings }, encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout).services;
}

test('PostgreSQL and Redis can be configured without IAM credentials', (t) => {
  const services = composeConfig(t, { SPX_REDIS_PASSWORD: '' }, ['postgresql', 'redis']);
  assert.deepEqual(Object.keys(services).sort(), ['postgresql', 'redis']);
  assert.equal(services.redis.environment.REDISCLI_AUTH, '');
});

test('bootstrap credentials are scoped to administrator initialization', (t) => {
  const settings = {
    SPX_IAM_BOOTSTRAP_USERNAME: 'config_admin',
    SPX_IAM_BOOTSTRAP_PASSWORD: 'fixture bootstrap password',
    SPX_IAM_CSRF_KEY: 'c'.repeat(32),
    SPX_CONSOLE_IAM_TOKEN: 'x'.repeat(32),
    SPX_INGEST_IAM_TOKEN: 'y'.repeat(32),
  };
  const services = composeConfig(t, settings);
  const iam = services.iam.environment;
  const initialization = services['iam-initialize'].environment;
  for (const [key, value] of Object.entries(settings)) {
    assert.equal(initialization[key], value);
    assert.equal(services['iam-migrate'].environment[key], undefined);
    assert.equal(iam[key], key.startsWith('SPX_IAM_BOOTSTRAP_') ? undefined : value);
  }
  assert.equal(iam.SPX_IAM_BIND_HOST, '0.0.0.0');
  assert.equal(services.console.environment.SPX_CONSOLE_BIND_HOST, '0.0.0.0');
  assert.equal(services.web.environment.SPX_IAM_HOST, 'iam');
  assert.equal(services.web.environment.SPX_CONSOLE_HOST, 'console');
});

test('the Web build can copy its public environment template', () => {
  const result = spawnSync('docker', ['build', '--output=type=cacheonly', '-f', '-', '.'], {
    cwd: root, encoding: 'utf8', input: 'FROM scratch\nCOPY src/apps/web/.env.example /web-env.example\n',
  });
  assert.equal(result.status, 0, result.stderr);
});

test('the full stack waits for Kafka topics, the ClickHouse schema and Ingest', (t) => {
  const services = composeConfig(t, { SPX_KAFKA_RETENTION_DAYS: '3', SPX_WEB_EDGE_TOKEN: 'a'.repeat(64) });
  assert.equal(services['event-topics'].environment.SPX_KAFKA_RETENTION_DAYS, '3');
  assert.equal(services['store-events'].depends_on['clickhouse-migrate'].condition, 'service_completed_successfully');
  assert.equal(services.ingest.depends_on['event-topics'].condition, 'service_completed_successfully');
  assert.equal(services.web.depends_on.ingest.condition, 'service_healthy');
  assert.equal(services.web.environment.SPX_WEB_EDGE_TOKEN, 'a'.repeat(64));
  assert.equal(services.ingest.environment.SPX_INGEST_CLICKHOUSE_MIGRATION_PASSWORD, undefined);
  assert.equal(services['store-events'].environment.SPX_INGEST_CLICKHOUSE_MIGRATION_PASSWORD, undefined);
});

test('changing the published Web port also changes the default browser origin', (t) => {
  const services = composeConfig(t, { SPX_WEB_PORT: '15173' });
  assert.equal(services.web.ports[0].published, '15173');
  assert.equal(services.iam.environment.SPX_WEB_ORIGIN, 'http://127.0.0.1:15173');
});

test('Compose applies supported Ingest capacity overrides to its server', (t) => {
  const settings = {
    SPX_INGEST_HTTP_MAX_BODY_BYTES: '2097152',
    SPX_INGEST_HTTP_MAX_CONCURRENT_REQUESTS: '31',
    SPX_INGEST_HTTP_RATE_PER_SEC: '12.5',
    SPX_INGEST_HTTP_RATE_BURST: '30',
    SPX_INGEST_MAX_IN_FLIGHT_BYTES: '4194304',
    SPX_INGEST_INPUT_KAFKA_RELOAD_INTERVAL: '40s',
    SPX_INGEST_SYSLOG_RELOAD_INTERVAL: '15s',
    SPX_INGEST_WORKERS: '0',
    SPX_INGEST_QUEUE_DEPTH: '2500',
    SPX_INGEST_BATCH_SIZE: '100',
    SPX_INGEST_FLUSH_INTERVAL: '750ms',
  };
  const { ingest } = composeConfig(t, settings);
  for (const [key, value] of Object.entries(settings)) assert.equal(ingest.environment[key], value);
});

test('the Ingest log level reaches the server and event writer', (t) => {
  const services = composeConfig(t, { SPX_INGEST_LOG_LEVEL: 'warn' });
  for (const service of ['ingest', 'store-events']) {
    assert.equal(services[service].environment.SPX_INGEST_LOG_LEVEL, 'warn');
    assert.equal(services[service].environment.SPX_INGEST_LOG_FILE, '');
  }
});

test('built-in Compose keeps service discovery, TLS termination and container logging', (t) => {
  const services = composeConfig(t, {
    SPX_POSTGRESQL_URL: 'postgresql://external.example/secplexus',
    SPX_REDIS_URL: 'rediss://external.example/0',
    SPX_KAFKA_BROKERS: 'external.example:9093',
    SPX_INGEST_TLS_CERT_FILE: '/path/to/test.pem',
    SPX_CLICKHOUSE_URL: 'https://external.example:8123',
    SPX_INGEST_LOG_FILE: 'logs/test.log',
  });
  const { environment } = services.ingest;
  assert.equal(environment.SPX_POSTGRESQL_HOST, 'postgresql');
  assert.equal(environment.SPX_REDIS_HOST, 'redis');
  assert.equal(environment.SPX_KAFKA_BROKERS, 'kafka:9092');
  assert.equal(environment.SPX_CLICKHOUSE_URL, 'http://clickhouse:8123');
  assert.equal(environment.SPX_POSTGRESQL_URL, undefined);
  assert.equal(environment.SPX_REDIS_URL, undefined);
  assert.equal(environment.SPX_INGEST_TLS_CERT_FILE, undefined);
  assert.equal(environment.SPX_INGEST_LOG_FILE, '');
});

test('IAM authentication policy and the Kafka namespace reach their Compose consumers', (t) => {
  const services = composeConfig(t, {
    SPX_IAM_LOGIN_MAX_FAILURES: '7',
    SPX_IAM_LOGIN_LOCK_DURATION: '20m',
    SPX_KAFKA_NAMESPACE: 'compose-test',
  });
  assert.equal(services.iam.environment.SPX_IAM_LOGIN_MAX_FAILURES, '7');
  assert.equal(services.iam.environment.SPX_IAM_LOGIN_LOCK_DURATION, '20m');
  for (const service of ['event-topics', 'store-events', 'ingest']) {
    assert.equal(services[service].environment.SPX_KAFKA_NAMESPACE, 'compose-test');
  }
});
