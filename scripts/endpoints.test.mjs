import assert from 'node:assert/strict';
import { test } from 'node:test';
import { endpoint, consoleEndpoint, webEndpoints } from './endpoints.mjs';

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
