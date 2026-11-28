import { isIP } from 'node:net';

// Listen addresses can use wildcards; clients need a concrete destination.
export function endpoint(address, scheme = 'http') {
  const url = new URL(`${scheme}://${address}`);
  if (['0.0.0.0', '[::]'].includes(url.hostname)) url.hostname = '127.0.0.1';
  return url.origin;
}

// A connection target is shared by callers, independently from its local bind.
export function serviceConnection(environment, module, local) {
  const prefix = `SPX_${module.toUpperCase()}`;
  const discovered = local && new URL(endpoint(local.public_addr, local.scheme));
  let host = environment[`${prefix}_HOST`] || discovered?.hostname || '127.0.0.1';
  if (host?.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1);
  const localPort = discovered && (discovered.port || (discovered.protocol === 'https:' ? '443' : '80'));
  const port = Number(environment[`${prefix}_PORT`] || localPort);
  if (!host || ['0.0.0.0', '::'].includes(host) || !Number.isInteger(port) || port < 1 || port > 65535)
    throw new Error(`${prefix}_HOST / ${prefix}_PORT connection must specify a concrete host and an integer port from 1 to 65535.`);
  const url = new URL(`http://${isIP(host) === 6 ? `[${host}]` : host}:${port}`);
  if (url.username || url.password || url.pathname !== '/' || url.search || url.hash || (!isIP(host) && url.hostname.toLowerCase() !== host.toLowerCase()))
    throw new Error(`${prefix}_HOST must be an IP address or hostname without a URL scheme, path or credentials.`);
  return { [`${prefix}_HOST`]: host, [`${prefix}_PORT`]: String(port) };
}

export function webEndpoints(environment, services) {
  const endpoints = { ...serviceConnection(environment, 'iam', services.iam), ...serviceConnection(environment, 'console', services.console) };
  if (!environment.SPX_INGEST_ORIGIN && (services.ingest || environment.SPX_INGEST_HOST || environment.SPX_INGEST_PORT)) {
    const { SPX_INGEST_HOST: host, SPX_INGEST_PORT: port } = serviceConnection(environment, 'ingest', services.ingest);
    endpoints.SPX_INGEST_ORIGIN = endpoint(`${isIP(host) === 6 ? `[${host}]` : host}:${port}`, services.ingest?.scheme);
  }

  return endpoints;
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
  const url = new URL(environment.SPX_WEB_ORIGIN || `http://127.0.0.1:${webListener(environment).port}`);
  if (!['http:', 'https:'].includes(url.protocol) || url.pathname !== '/' || url.search || url.hash || url.username || url.password)
    throw new Error('SPX_WEB_ORIGIN must be an HTTP(S) origin without a path, query or credentials.');
  return url.origin;
}
