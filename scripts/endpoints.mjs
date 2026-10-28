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
