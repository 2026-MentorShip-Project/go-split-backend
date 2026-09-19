import { appendFile, readFile } from 'node:fs/promises';
const pkg = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const url = `https://registry.npmjs.org/${encodeURIComponent(pkg.name)}/${encodeURIComponent(pkg.version)}`;
const response = await fetch(url, { signal: AbortSignal.timeout(30000) });
if (response.status !== 200 && response.status !== 404) {
  throw new Error(`Registry check failed: HTTP ${response.status}; refusing to guess whether this version exists`);
}
const exists = response.status === 200;
if (exists) {
  const published = await response.json();
  if (published.name !== pkg.name || published.version !== pkg.version) throw new Error('Unexpected registry response');
}
await appendFile(process.env.GITHUB_OUTPUT, `exists=${exists}\n`);
console.log(exists ? `${pkg.name}@${pkg.version} already exists; not overwriting` : `${pkg.name}@${pkg.version} is ready to publish`);
