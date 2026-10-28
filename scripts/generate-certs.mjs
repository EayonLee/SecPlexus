import { existsSync, mkdirSync, chmodSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('..', import.meta.url));
const directory = path.join(root, 'certs');
try {
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const key = path.join(directory, 'iam.key');
  const cert = path.join(directory, 'iam.crt');
  if (existsSync(key) !== existsSync(cert)) throw new Error('TLS certificate/key pair is incomplete; inspect certs/ before continuing.');
  if (!existsSync(key)) {
    const result = spawnSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', key, '-out', cert, '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1,DNS:localhost,DNS:iam', '-days', '365'], { stdio: 'ignore' });
    if (result.status !== 0) throw new Error('Certificate generation failed; install OpenSSL or provide your own TLS pair.');
    chmodSync(key, 0o600);
  }
  console.log('Internal TLS certificates are ready in certs/; existing certificates were preserved.');
} catch (error) { console.error(error.message); process.exitCode = 1; }
