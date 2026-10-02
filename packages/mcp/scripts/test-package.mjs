import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const tarball = resolve(process.argv[2]);
const dir = mkdtempSync(join(tmpdir(), 'go-split-mcp-'));
writeFileSync(join(dir, 'package.json'), '{"private":true}');
execFileSync('npm', ['install', '--no-audit', '--no-fund', tarball], { cwd: dir, stdio: 'inherit' });

const run = spawnSync(join(dir, 'node_modules/.bin/go-split-mcp'), { env: { PATH: process.env.PATH }, encoding: 'utf8' });
if (run.status !== 1 || !run.stderr.includes('GO_SPLIT_TOKEN is not set')) {
  throw new Error(`installed bin did not start as expected: status ${run.status}, stderr ${run.stderr}`);
}
console.log('installed bin starts and asks for GO_SPLIT_TOKEN');
