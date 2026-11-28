import assert from 'node:assert/strict';
import { test } from 'node:test';
import { integrationEnvironment, testDependencies, testPostgresURL } from '../tests/integration/dependencies.mjs';

const endpoints = {
  SPX_TEST_POSTGRESQL_URL: 'postgres://localhost/test',
  SPX_TEST_REDIS_URL: 'redis://localhost/4',
  SPX_TEST_KAFKA_BROKERS: 'localhost:9092',
};

test('external suites use only explicit test endpoints and prefer process overrides', () => {
  const local = { ...endpoints, SPX_POSTGRESQL_URL: 'postgres://dev', SPX_IAM_TOKEN: 'private' };
  assert.deepEqual(testDependencies({}, local, { SPX_TEST_REDIS_URL: 'redis://another/0' }), {
    ...endpoints, SPX_TEST_REDIS_URL: 'redis://another/0',
  });
});

test('missing Kafka fails before provisioning even when the application has a broker', () => {
  assert.throws(() => testDependencies({}, endpoints, {
    SPX_TEST_KAFKA_BROKERS: '', SPX_KAFKA_BROKERS: 'application:9092',
  }), /SPX_TEST_KAFKA_BROKERS/);
});

test('ClickHouse backup verification uses the file setting and honors process overrides', () => {
  const local = { ...endpoints, SPX_TEST_CLICKHOUSE_URL: 'http://localhost:8123', SPX_TEST_CLICKHOUSE_BACKUPS: '1' };
  assert.equal(testDependencies({ withClickHouse: true }, local, {}).SPX_TEST_CLICKHOUSE_BACKUPS, '1');
  assert.equal(testDependencies({ withClickHouse: true }, local, { SPX_TEST_CLICKHOUSE_BACKUPS: '0' }).SPX_TEST_CLICKHOUSE_BACKUPS, '0');
});

test('ClickHouse settings are ignored unless the suite opts in', () => {
  const local = { ...endpoints, SPX_TEST_CLICKHOUSE_URL: 'http://localhost:8123' };
  const environment = {
    SPX_TEST_CLICKHOUSE_URL: 'http://override:8123',
    SPX_TEST_CLICKHOUSE_BACKUPS: '1',
    SPX_TEST_REQUIRE_CLICKHOUSE: '1',
  };
  assert.deepEqual(testDependencies({}, local, environment), endpoints);
  assert.deepEqual(integrationEnvironment({ useDocker: false, withClickHouse: false }, local, environment), endpoints);
  assert.deepEqual(integrationEnvironment({ useDocker: true, withClickHouse: false }, local, environment), {});
  assert.deepEqual(integrationEnvironment({ useDocker: true, withClickHouse: true }, local, environment), {});
  assert.deepEqual(integrationEnvironment({ useDocker: false, withClickHouse: true }, local, environment), {
    ...endpoints, SPX_TEST_CLICKHOUSE_URL: 'http://override:8123', SPX_TEST_CLICKHOUSE_BACKUPS: '1',
  });
});

test('SQL preparation requires only an explicit test PostgreSQL URL', () => {
  const local = { SPX_TEST_POSTGRESQL_URL: endpoints.SPX_TEST_POSTGRESQL_URL };
  assert.equal(testPostgresURL(local, {}), endpoints.SPX_TEST_POSTGRESQL_URL);
  assert.equal(testPostgresURL(local, { SPX_TEST_POSTGRESQL_URL: 'postgres://localhost/overridden' }),
    'postgres://localhost/overridden');
  assert.throws(() => testPostgresURL(local, { SPX_TEST_POSTGRESQL_URL: '' }), /SPX_TEST_POSTGRESQL_URL/);
});
