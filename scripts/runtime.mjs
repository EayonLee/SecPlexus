import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';

const relative = path.join('.spx', 'run.json');

/** Path of the per-checkout process registry. */
export function runtimeFile(root) {
  return path.join(root, relative);
}

/** Whether a process id still belongs to a live process. */
export function processAlive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return error.code === 'EPERM';
  }
}

/** Read registered service roots; missing or malformed state is treated as empty. */
export function readRuntime(root) {
  const filename = runtimeFile(root);
  if (!existsSync(filename)) return [];
  try {
    const value = JSON.parse(readFileSync(filename, 'utf8'));
    if (!Array.isArray(value.processes)) return [];
    return value.processes.filter((entry) =>
      entry && Number.isInteger(entry.pid) && entry.pid > 0 && typeof entry.module === 'string',
    );
  } catch {
    return [];
  }
}

/** 检查工作目录是否已有服务占用，必须在构建或申请测试资源前执行。 */
export function assertStackAvailable(root, includesWeb) {
  const running = readRuntime(root).filter((entry) => processAlive(entry.pid));
  if (running.length)
    throw new Error(`Another SecPlexus stack is already running (PID ${running.map((entry) => entry.pid).join(', ')}). Use a separate checkout to run both stacks.`);
  if (!includesWeb) return;
  let lock;
  try {
    lock = JSON.parse(readFileSync(path.join(root, 'src/apps/web/.next/dev/lock'), 'utf8'));
  } catch {
    return;
  }
  if (processAlive(lock?.pid))
    throw new Error(`A Web dev server is already running (PID ${lock.pid}). Use a separate checkout to run both stacks.`);
}

/** Replace the registry atomically so a concurrent stop never reads half a record. */
export function writeRuntime(root, processes) {
  const filename = runtimeFile(root);
  mkdirSync(path.dirname(filename), { recursive: true });
  const temporary = `${filename}.${process.pid}.tmp`;
  writeFileSync(temporary, JSON.stringify({ version: 1, processes }, null, 2) + '\n');
  try {
    renameSync(temporary, filename);
  } catch (error) {
    rmSync(temporary, { force: true });
    throw error;
  }
}

/** Remove the registry after a clean shutdown. */
export function clearRuntime(root) {
  rmSync(runtimeFile(root), { force: true });
}
