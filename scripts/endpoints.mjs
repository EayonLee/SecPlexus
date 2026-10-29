import { isIP } from 'node:net';

// Listen addresses can use wildcards; clients need a concrete destination.
export function endpoint(address, protocol = 'http') {
  const url = new URL(`${protocol}://${address}`);
  if (['0.0.0.0', '[::]'].includes(url.hostname)) url.hostname = '127.0.0.1';
  return url.origin;
}

// Root startup knows the actual Go configuration. Standalone callers can still
// supply an explicit remote URL through the owning module's configuration.
export function consoleEndpoint(environment, iam) {
  return environment.SPX_CONSOLE_IAM_BASE_URL || endpoint(iam.internal_addr, 'https');
}
export function webEndpoints(environment, services) {
  return {
    SPX_WEB_IAM_PROXY_TARGET: environment.SPX_WEB_IAM_PROXY_TARGET || endpoint(services.iam.public_addr),
    SPX_WEB_CONSOLE_PROXY_TARGET: environment.SPX_WEB_CONSOLE_PROXY_TARGET || endpoint(services.console.public_addr),
  };
}

// Native Next owns the browser listener; the public origin is its port authority.
export function webListener(environment) {
  const url = new URL(environment.SPX_PUBLIC_URL || 'http://127.0.0.1:5173');
  if (url.protocol !== 'http:') throw new Error('Local Next startup requires an HTTP public URL.');
  if (url.pathname !== '/' || url.search || url.hash || url.username || url.password)
    throw new Error('SPX_PUBLIC_URL must be an origin without a path, query or credentials.');
  const host = environment.SPX_WEB_HOST || '127.0.0.1';
  const port = Number(url.port || 80);
  if (host !== 'localhost' && !isIP(host)) throw new Error('SPX_WEB_HOST must be an IP address or localhost.');
  if (port === 0) throw new Error('The public browser port must be between 1 and 65535.');
  return { host, port };
}
