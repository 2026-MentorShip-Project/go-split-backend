import { execFileSync } from 'node:child_process';
import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = new URL('../../../', import.meta.url);
const pkg = new URL('../', import.meta.url);
const manifest = JSON.parse(await readFile(new URL('package.json', pkg), 'utf8'));
const source = await readFile(new URL('internal/splitengine/engine.go', root), 'utf8');
const engineVersion = source.match(/^const Version = "([^"]+)"$/m)?.[1];
if (engineVersion !== manifest.version) throw new Error('package.json version must match splitengine.Version');
const dist = new URL('dist/', pkg);
await mkdir(dist, { recursive: true });
execFileSync('go', ['build', '-trimpath', '-buildvcs=false', '-o', fileURLToPath(new URL('engine.wasm', dist)), './cmd/splitengine-wasm'], {
  cwd: root, env: { ...process.env, GOOS: 'js', GOARCH: 'wasm', CGO_ENABLED: '0' }, stdio: 'inherit',
});
const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8', cwd: root }).trim();
await copyFile(`${goroot}/lib/wasm/wasm_exec.js`, new URL('wasm_exec.js', dist));
// Preserve the Go runtime's redistribution notice alongside the copied runtime.
let license;
try { license = await readFile(`${goroot}/LICENSE`); }
catch (error) {
  if (error.code !== 'ENOENT') throw error;
  // Homebrew places the distribution license alongside libexec/GOROOT.
  license = await readFile(`${goroot}/../LICENSE`);
}
await writeFile(new URL('GO-LICENSE', dist), license);
for (const name of ['index.js', 'index.d.ts']) await copyFile(new URL(`src/${name}`, pkg), new URL(name, dist));
await writeFile(new URL('version.js', dist), `export const engineVersion = ${JSON.stringify(engineVersion)};\n`);
