import assert from 'node:assert/strict';
import { test } from 'node:test';
import { endpoint, consoleEndpoint, webEndpoints, webListener } from './endpoints.mjs';

test('client endpoints use actual module ports and normalize wildcard bindings', () => {
  const services = { iam: { public_addr: '0.0.0.0:18081', internal_addr: '[::]:19081' }, console: { public_addr: '127.0.0.1:18080' } };
  assert.equal(consoleEndpoint({}, services.iam), 'https://127.0.0.1:19081');
  assert.deepEqual(webEndpoints({},services), { SPX_WEB_IAM_PROXY_TARGET:'http://127.0.0.1:18081', SPX_WEB_CONSOLE_PROXY_TARGET:'http://127.0.0.1:18080' });
  assert.equal(endpoint('[::1]:8081'),'http://[::1]:8081');
});
test('explicit remote endpoints override local discovery', () => {
  const services = { iam: {public_addr:'0.0.0.0:8081',internal_addr:'0.0.0.0:9081'},console:{public_addr:'0.0.0.0:8080'} };
  assert.equal(consoleEndpoint({SPX_CONSOLE_IAM_BASE_URL:'https://iam.example:9443'},services.iam),'https://iam.example:9443');
  assert.equal(webEndpoints({SPX_WEB_IAM_PROXY_TARGET:'https://api.example'},services).SPX_WEB_IAM_PROXY_TARGET,'https://api.example');
});

test('Web listens directly on the public URL port with an explicit bind host', () => {
  assert.deepEqual(webListener({ SPX_PUBLIC_URL: 'http://localhost:15173', SPX_WEB_HOST: '0.0.0.0' }), { host: '0.0.0.0', port: 15173 });
  assert.deepEqual(webListener({ SPX_PUBLIC_URL: 'http://localhost' }), { host: '127.0.0.1', port: 80 });
  assert.throws(() => webListener({ SPX_PUBLIC_URL: 'https://localhost:15173' }), /HTTP/);
  assert.throws(() => webListener({ SPX_PUBLIC_URL: 'http://localhost:15173/subpath' }), /origin/);
});

test('Web rejects invalid binding and zero browser port before starting services', () => {
  assert.throws(() => webListener({ SPX_PUBLIC_URL:'http://localhost:5173',SPX_WEB_HOST:'invalid hostname' }), /host/);
  assert.throws(() => webListener({ SPX_PUBLIC_URL:'http://localhost:0' }), /port/);
});
