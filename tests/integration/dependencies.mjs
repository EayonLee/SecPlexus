import { readFileSync } from 'node:fs';
import { parseEnv } from 'node:util';

function localSettings() {
  try {
    return parseEnv(readFileSync(new URL('../../.env', import.meta.url), 'utf8'));
  } catch (error) {
    if (error.code === 'ENOENT') return {};
    throw error;
  }
}

export function testPostgresURL(local = localSettings(), environment = process.env) {
  const url = environment.SPX_TEST_POSTGRESQL_URL ?? local.SPX_TEST_POSTGRESQL_URL;
  if (!url?.trim()) throw new Error('SPX_TEST_POSTGRESQL_URL is required. Set it in .env or the process environment.');
  return url;
}

export function testDependencies({ withClickHouse = false } = {}, local = localSettings(), environment = process.env) {
  const keys = ['SPX_TEST_POSTGRESQL_URL', 'SPX_TEST_REDIS_URL', 'SPX_TEST_KAFKA_BROKERS'];
  const entries = keys.map((key) => [key, environment[key] ?? local[key]]);
  const missing = entries.filter(([, value]) => !value?.trim()).map(([key]) => key);
  if (missing.length) throw new Error(`Missing test settings: ${missing.join(', ')}. Set them in .env or the process environment, or explicitly pass --docker.`);
  if (!withClickHouse) return Object.fromEntries(entries);
  const clickhouse = environment.SPX_TEST_CLICKHOUSE_URL ?? local.SPX_TEST_CLICKHOUSE_URL;
  if (clickhouse?.trim()) entries.push(['SPX_TEST_CLICKHOUSE_URL', clickhouse]);
  const backups = environment.SPX_TEST_CLICKHOUSE_BACKUPS ?? local.SPX_TEST_CLICKHOUSE_BACKUPS;
  if (backups) {
    if (backups !== '0' && backups !== '1') throw new Error('SPX_TEST_CLICKHOUSE_BACKUPS must be 0 or 1.');
    entries.push(['SPX_TEST_CLICKHOUSE_BACKUPS', backups]);
  }
  return Object.fromEntries(entries);
}

export function integrationEnvironment({ useDocker, withClickHouse }, local, environment = process.env) {
  const env = { ...environment };
  delete env.SPX_TEST_CLICKHOUSE_URL;
  delete env.SPX_TEST_CLICKHOUSE_BACKUPS;
  delete env.SPX_TEST_REQUIRE_CLICKHOUSE;
  if (!useDocker) {
    delete env.SPX_TEST_POSTGRESQL_CONTAINER;
    Object.assign(env, testDependencies({ withClickHouse }, local, environment));
  }
  return env;
}
