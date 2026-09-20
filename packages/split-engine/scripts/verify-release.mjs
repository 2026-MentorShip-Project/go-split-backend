import { readFile } from 'node:fs/promises';
const pkg = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const tag = process.env.GITHUB_REF_NAME;
if (!/^engine-v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(tag ?? '') || tag !== `engine-v${pkg.version}`) {
  throw new Error(`Release must run from tag engine-v${pkg.version}, got ${tag}`);
}
const source = await readFile(new URL('../../../internal/splitengine/engine.go', import.meta.url), 'utf8');
if (!source.includes(`const Version = "${pkg.version}"`)) throw new Error('Go engine and npm package versions must match');
console.log(`Releasing ${pkg.name}@${pkg.version}`);
