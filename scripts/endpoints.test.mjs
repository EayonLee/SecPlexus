import assert from 'node:assert/strict';
import { test } from 'node:test';
import { endpoint, serviceConnection, webEndpoints, webListener, webOrigin } from './endpoints.mjs';

test('local discovery converts wildcard listeners into usable shared service locations', () => {
 const services={iam:{public_addr:'0.0.0.0:18081'},console:{public_addr:'[::]:18080'}};
 assert.deepEqual(webEndpoints({},services),{SPX_IAM_HOST:'127.0.0.1',SPX_IAM_PORT:'18081',SPX_CONSOLE_HOST:'127.0.0.1',SPX_CONSOLE_PORT:'18080'});
 assert.equal(endpoint('[::1]:8081'),'http://[::1]:8081');
});

test('one remote IAM location reaches all callers without changing local bind configuration',()=>{
 const env={SPX_IAM_HOST:'iam.corp',SPX_IAM_PORT:'9443',SPX_CONSOLE_HOST:'10.0.0.20',SPX_CONSOLE_PORT:'8080'};
 const shared=serviceConnection(env,'iam',{public_addr:'0.0.0.0:8081'});
 assert.deepEqual(shared,{SPX_IAM_HOST:'iam.corp',SPX_IAM_PORT:'9443'});
 assert.deepEqual(webEndpoints(env,{}),{...shared,SPX_CONSOLE_HOST:'10.0.0.20',SPX_CONSOLE_PORT:'8080'});
});

test('Web listening port is independent from the external HTTPS origin',()=>{
 assert.deepEqual(webListener({SPX_WEB_PORT:'15173',SPX_WEB_ORIGIN:'https://sec.example.com'}),{host:'0.0.0.0',port:15173});
 assert.equal(webOrigin({SPX_WEB_PORT:'15173',SPX_WEB_ORIGIN:'https://sec.example.com'}),'https://sec.example.com');
 assert.equal(webOrigin({SPX_WEB_PORT:'15173'}),'http://localhost:15173');
});

test('invalid binding, port, external origin and unusable connection addresses fail at loading',()=>{
 for(const value of ['0','65536','invalid','']) assert.throws(()=>webListener({SPX_WEB_PORT:value}),/port/i);
 assert.throws(()=>webListener({SPX_WEB_PORT:'5173',SPX_WEB_BIND_HOST:'not a host'}),/host/i);
 assert.throws(()=>webOrigin({SPX_WEB_PORT:'5173',SPX_WEB_ORIGIN:'https://sec.example.com/path'}),/origin/i);
 assert.throws(()=>serviceConnection({SPX_IAM_HOST:'0.0.0.0',SPX_IAM_PORT:'8081'},'iam'),/connection/i);
 assert.throws(()=>serviceConnection({},'iam'),/connection/i);
});
