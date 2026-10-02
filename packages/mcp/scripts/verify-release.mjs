import { readFile } from 'node:fs/promises';
const pkg = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const tag = process.env.GITHUB_REF_NAME;
if (!/^mcp-v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(tag ?? '') || tag !== `mcp-v${pkg.version}`) {
  throw new Error(`Release must run from tag mcp-v${pkg.version}, got ${tag}`);
}
console.log(`Releasing ${pkg.name}@${pkg.version}`);
