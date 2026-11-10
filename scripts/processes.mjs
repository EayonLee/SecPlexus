import { spawnSync } from 'node:child_process';

/**
 * Terminate a development process and all of its descendants.
 *
 * Windows has no process-group semantics. Killing only the direct child leaves
 * the Next or Go process under the pnpm/cmd wrapper alive, still holding its
 * port and blocking the next startup.
 */
export async function terminateProcessTree(child) {
  if (child.exitCode !== null || child.signalCode !== null || !child.pid) return;
  function signal(name) {
    if (process.platform === 'win32') {
      spawnSync('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
      return;
    }
    process.kill(-child.pid, name);
  }
  try {
    signal('SIGTERM');
  } catch (error) {
    if (error.code !== 'ESRCH') throw error;
  }
  if (child.exitCode !== null || child.signalCode !== null) return;
  let timeout;
  await Promise.race([
    new Promise((resolve) => child.once('exit', resolve)),
    new Promise((resolve) => {
      timeout = setTimeout(resolve, 5000);
    }),
  ]);
  clearTimeout(timeout);
  if (child.exitCode === null && child.signalCode === null) {
    try {
      signal('SIGKILL');
    } catch (error) {
      if (error.code !== 'ESRCH') throw error;
    }
  }
}
