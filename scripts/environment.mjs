import { chmodSync, constants, copyFileSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { parseEnv } from 'node:util';

export function loadEnvironment(root, environment = process.env) {
  const filename = path.join(root, '.env');
  let content;
  try {
    content = readFileSync(filename, 'utf8');
  } catch (error) {
    if (error.code !== 'ENOENT') throw new Error('Cannot read .env. Check its file permissions.');
    try {
      copyFileSync(path.join(root, '.env.example'), filename, constants.COPYFILE_EXCL);
      chmodSync(filename, 0o600);
    } catch (copyError) {
      if (copyError.code === 'EEXIST') return loadEnvironment(root, environment);
      throw new Error('Cannot create .env. Copy .env.example to .env and configure it before starting.');
    }
    throw new Error('Created .env from .env.example. Review the settings, then run the startup command again.');
  }
  const values = { ...parseEnv(content), ...environment };
  // Root .env paths are relative to the checkout; modules receive absolute paths.
  for (const [key, value] of Object.entries(values)) {
    if (key.startsWith('SPX_') && key.endsWith('_FILE') && value) values[key] = path.resolve(root, value);
  }
  return values;
}

export function webEnvironment(environment) {
  const values = Object.fromEntries(Object.entries(environment).filter(([key]) =>
    !key.startsWith('SPX_') || key.startsWith('SPX_WEB_')));
  if (environment.SPX_PUBLIC_URL) values.NEXT_PUBLIC_APP_URL = environment.NEXT_PUBLIC_APP_URL || environment.SPX_PUBLIC_URL;
  return values;
}

export function moduleEnvironment(environment, module, initializing = false) {
  const allowed = new Set(module === 'iam' ? ['SPX_PUBLIC_URL', 'SPX_CONSOLE_IAM_TOKEN'] : []);
  return Object.fromEntries(Object.entries(environment).filter(([key]) => {
    if (!key.startsWith('SPX_')) return !key.startsWith('NEXT_PUBLIC_');
    if (allowed.has(key)) return true;
    if (module === 'iam' && (key.startsWith('SPX_POSTGRES_') || key.startsWith('SPX_REDIS_'))) return true;
    if (!key.startsWith(`SPX_${module.toUpperCase()}_`)) return false;
    return initializing || !key.startsWith('SPX_IAM_BOOTSTRAP_');
  }));
}
