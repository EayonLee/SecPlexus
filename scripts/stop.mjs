import { spawnSync } from 'node:child_process';
import { existsSync, readdirSync, readlinkSync, realpathSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

const root = realpathSync(fileURLToPath(new URL('..', import.meta.url)));
const apps = path.join(root, 'src/apps');

function run(command, args) {
  const result = spawnSync(command, args, { encoding: 'utf8', timeout: 15_000 });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} failed: ${result.stderr.trim() || `exit ${result.status}`}`);
  }
  return result.stdout;
}

function processes() {
  const output = run('ps', ['-u', String(process.getuid()), '-o', 'pid=,ppid=,stat=,args=']);
  const result = new Map();
  for (const line of output.split('\n')) {
    const match = line.match(/^\s*(\d+)\s+(\d+)\s+(\S+)\s+(.*)$/);
    if (match && !match[3].startsWith('Z')) {
      result.set(Number(match[1]), { parent: Number(match[2]), command: match[4] });
    }
  }
  return result;
}

function workingDirectories(pids) {
  const result = new Map();
  if (process.platform === 'linux') {
    for (const pid of pids) {
      try { result.set(pid, readlinkSync(`/proc/${pid}/cwd`)); }
      catch (error) { if (!['ENOENT', 'EACCES', 'ESRCH'].includes(error.code)) throw error; }
    }
  } else {
    const output = spawnSync('lsof', ['-a', '-p', pids.join(','), '-d', 'cwd', '-Fn'], { encoding: 'utf8', timeout: 15_000 });
    if (output.error) throw output.error;
    if (![0, 1].includes(output.status)) throw new Error('Cannot inspect process working directories.');
    let pid;
    for (const line of output.stdout.split('\n')) {
      if (line.startsWith('p')) pid = Number(line.slice(1));
      if (line.startsWith('n')) result.set(pid, line.slice(1));
    }
  }
  return result;
}

const inside = (directory, parent) => directory === parent || directory.startsWith(`${parent}${path.sep}`);

async function stopProcesses() {
  const snapshot = processes();
  const protectedPids = new Set();
  for (let pid = process.pid; snapshot.has(pid); pid = snapshot.get(pid).parent) protectedPids.add(pid);

  const listeners = spawnSync('lsof', ['-nP', '-iTCP', '-sTCP:LISTEN', '-Fp'], { encoding: 'utf8', timeout: 15_000 });
  if (listeners.error) throw listeners.error;
  if (![0, 1].includes(listeners.status)) throw new Error('Cannot inspect running service ports.');
  const pids = [...new Set([...listeners.stdout.matchAll(/^p(\d+)$/gm)].map(match => Number(match[1])))];
  if (!pids.length) return 0;
  const directories = workingDirectories([...snapshot.keys()]);
  function belongsToProject(pid) {
    const visited = new Set();
    while (snapshot.has(pid) && !protectedPids.has(pid) && !visited.has(pid)) {
      visited.add(pid);
      const directory = directories.get(pid);
      const { command, parent } = snapshot.get(pid);
      if (directory && inside(directory, apps)) return true;
      if (directory && inside(directory, root) &&
          (command.includes(`${apps}/`) || command.includes('src/apps/') || command.includes(`${root}/bin/`) || command.includes('scripts/start.mjs') || /(?:^|\s)(?:\.\/)?(?:bin\/(?:iam|console))(?:\s|$)/.test(command))) return true;
      pid = parent;
    }
    return false;
  }
  const selected = new Set(pids.filter(pid => !protectedPids.has(pid) && belongsToProject(pid)));
  if (!selected.size) return 0;
  // Include workers and subprocesses belonging to each service.
  let added;
  do {
    added = false;
    for (const [pid, info] of snapshot) {
      if (selected.has(info.parent) && !selected.has(pid) && !protectedPids.has(pid)) {
        selected.add(pid);
        added = true;
      }
    }
  } while (added);

  const remaining = () => {
    const current = processes();
    return [...selected].filter(pid => current.get(pid)?.command === snapshot.get(pid)?.command);
  };
  function signal(pids, name) {
    for (const pid of pids) {
      try { process.kill(pid, name); }
      catch (error) { if (error.code !== 'ESRCH') throw error; }
    }
  }
  console.log(`Stopping ${selected.size} project service process(es).`);
  signal(remaining(), 'SIGTERM');
  const deadline = Date.now() + 5_000;
  while (remaining().length && Date.now() < deadline) await delay(200);
  signal(remaining(), 'SIGKILL');
  await delay(100);
  if (remaining().length) throw new Error('Some project service processes could not be stopped.');
  return selected.size;
}

function hasComposeConfiguration(directory) {
  if (!existsSync(directory)) return false;
  return readdirSync(directory, { withFileTypes: true }).some(entry =>
    entry.isDirectory() ? hasComposeConfiguration(path.join(directory, entry.name)) :
      /^(?:docker-)?compose(?:[.-].*)?\.ya?ml$/.test(entry.name));
}

function stopContainers() {
  const configured = [root, path.join(root, 'deploy')].some(directory => {
    if (directory !== root) return hasComposeConfiguration(directory);
    return readdirSync(root).some(name => /^(?:docker-)?compose(?:[.-].*)?\.ya?ml$/.test(name));
  });
  if (!configured) return 0;
  const docker = spawnSync('docker', ['info', '--format', '{{.ServerVersion}}'], {
    encoding: 'utf8', timeout: 5_000,
  });
  if (docker.error || docker.status !== 0) return null;
  const output = run('docker', ['ps', '--filter', 'label=com.docker.compose.project.working_dir', '--format',
    '{{.ID}}\t{{.Label "com.docker.compose.project.working_dir"}}']);
  const ids = output.trim().split('\n').filter(Boolean).flatMap(line => {
    const [id, directory] = line.split('\t');
    return directory && inside(path.resolve(directory), root) ? [id] : [];
  });
  if (ids.length) {
    console.log(`Stopping ${ids.length} project container(s); data and volumes are retained.`);
    run('docker', ['stop', '--time', '10', ...ids]);
  }
  return ids.length;
}

let count = 0;
let failed = false;
let unknownContainers = false;
for (const stop of [stopProcesses, stopContainers]) {
  try {
    const stopped = await stop();
    if (stopped === null) unknownContainers = true;
    else count += stopped;
  }
  catch (error) { failed = true; console.error(error.message); }
}
if (failed) process.exitCode = 1;
else if (unknownContainers) console.log('Application stop completed; Docker container status is unknown.');
else console.log(count ? 'SecPlexus services stopped.' : 'No SecPlexus services are running.');
