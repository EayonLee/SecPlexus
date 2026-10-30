import { isIP } from 'node:net';

// Listen addresses can use wildcards; clients need a concrete destination.
export function endpoint(address) {
  const url = new URL(`http://${address}`);
  if (['0.0.0.0', '[::]'].includes(url.hostname)) url.hostname = '127.0.0.1';
  return url.origin;
}

// A connection target is shared by callers, independently from its local bind.
export function serviceConnection(environment, module, local) {
  const prefix = `SPX_${module.toUpperCase()}`;
  const discovered = local && new URL(endpoint(local.public_addr));
  let host = environment[`${prefix}_HOST`] || discovered?.hostname;
  if (host?.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1);
  const port = Number(environment[`${prefix}_PORT`] || discovered?.port);
  if (!host || ['0.0.0.0', '::'].includes(host) || !Number.isInteger(port) || port < 1 || port > 65535)
    throw new Error(`${prefix}_HOST / ${prefix}_PORT connection must specify a concrete host and an integer port from 1 to 65535.`);
  const url = new URL(`http://${isIP(host) === 6 ? `[${host}]` : host}:${port}`);
  if (url.username || url.password || url.pathname !== '/' || url.search || url.hash || (!isIP(host) && url.hostname.toLowerCase() !== host.toLowerCase()))
    throw new Error(`${prefix}_HOST must be an IP address or hostname without a URL scheme, path or credentials.`);
  return { [`${prefix}_HOST`]: host, [`${prefix}_PORT`]: String(port) };
}

export function webEndpoints(environment, services) {
  return { ...serviceConnection(environment, 'iam', services.iam), ...serviceConnection(environment, 'console', services.console) };
}

// The public reverse-proxy origin does not determine the local listening port.
export function webListener(environment) {
  const port = Number(environment.SPX_WEB_PORT);
  const host = environment.SPX_WEB_BIND_HOST || '0.0.0.0';
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('SPX_WEB_PORT must be between 1 and 65535.');
  if (host !== 'localhost' && !isIP(host)) throw new Error('SPX_WEB_BIND_HOST must be an IP address or localhost.');
  return { host, port };
}

export function webOrigin(environment) {
  const url = new URL(environment.SPX_WEB_ORIGIN || `http://localhost:${webListener(environment).port}`);
  if (!['http:', 'https:'].includes(url.protocol) || url.pathname !== '/' || url.search || url.hash || url.username || url.password)
    throw new Error('SPX_WEB_ORIGIN must be an HTTP(S) origin without a path, query or credentials.');
  return url.origin;
}
