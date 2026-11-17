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
  // 空值视为"未设置"：根 .env 是模板，未填写的密钥留空；
  // 若让空串覆盖显式传入的进程环境，YAML 的 ${VAR-} 不会再用默认值兜底，
  // 表现为"明明传了 token 却说缺失"。安全语义不变：缺失仍然拒绝启动。
  // 只对密钥类丢弃空值：模板里留空的 token/key 不应覆盖显式传入的值。
  // 其他键保留空值语义——例如把 *_LOG_FILE 设为空表示"不写文件"。
  const secretSuffixes = ['_TOKEN', '_KEY', '_PASSWORD', '_SECRET'];
  const values = {};
  for (const [key, value] of Object.entries({ ...parseEnv(content), ...environment })) {
    if (value === '' && key.startsWith('SPX_') && secretSuffixes.some((suffix) => key.endsWith(suffix))) continue;
    values[key] = value;
  }
  // Root .env paths are relative to the checkout; modules receive absolute paths.
  for (const [key, value] of Object.entries(values)) {
    if (key.startsWith('SPX_') && key.endsWith('_FILE') && value) values[key] = path.resolve(root, value);
  }
  return values;
}

export function webEnvironment(environment) {
  const allowed = new Set(['SPX_WEB_PORT','SPX_WEB_BIND_HOST','SPX_IAM_HOST','SPX_IAM_PORT','SPX_CONSOLE_HOST','SPX_CONSOLE_PORT']);
  return Object.fromEntries(Object.entries(environment).filter(([key]) =>
    key.startsWith('SPX_') ? allowed.has(key) : true));
}

export function moduleEnvironment(environment, module) {
  const allowed = new Set(module === 'iam' ? ['SPX_WEB_ORIGIN', 'SPX_CONSOLE_IAM_TOKEN'] : ['SPX_IAM_HOST','SPX_IAM_PORT']);
  return Object.fromEntries(Object.entries(environment).filter(([key]) => {
    if (!key.startsWith('SPX_')) return !key.startsWith('NEXT_PUBLIC_');
    if (allowed.has(key)) return true;
    if (module === 'iam' && (key.startsWith('SPX_POSTGRES_') || key.startsWith('SPX_REDIS_'))) return true;
    // IAM 需要校验每个 service client 的凭证：凡 <APP>_IAM_TOKEN 都属于它自己的配置。
    if (module === 'iam' && key.endsWith('_IAM_TOKEN')) return true;
    if (module === 'ingest' && (key.startsWith('SPX_REDIS_') || key.startsWith('SPX_KAFKA_'))) return true;
    if (!key.startsWith(`SPX_${module.toUpperCase()}_`)) return false;
    return true;
  }));
}
