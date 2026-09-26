import { execFileSync } from 'node:child_process';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
const tarball = resolve(process.argv[2]);
const dir = await mkdtemp(join(tmpdir(), 'go-split-npm-consumer-'));
try {
  await writeFile(join(dir, 'package.json'), JSON.stringify({ private: true, type: 'module' }));
  execFileSync('npm', ['install', '--offline', '--ignore-scripts', '--no-audit', '--no-fund', tarball], { cwd: dir, stdio: 'inherit' });
  execFileSync(process.execPath, ['--input-type=module', '-e', `
    import assert from 'node:assert/strict';
    import { readFile } from 'node:fs/promises';
    import { initEngine, splitDetail, validateRule, engineVersion } from '@go-split/engine';
    const wasmBytes = await readFile(new URL(import.meta.resolve('@go-split/engine/engine.wasm')));
    await initEngine({ wasmBytes });
    const result = splitDetail({detail:{amount:101},members:[{id:1},{id:2},{id:3}]});
    assert.deepEqual(result.shares.map(s=>s.amount), [34,34,33]);
    const accepted = validateRule({groups:[{conds:['a'],mode:'weight',weight:0.5}],cond_tags:['a']});
    assert.equal(accepted.ok, true);
    assert.equal(validateRule({groups:[{conds:[],mode:'exclude'}],cond_tags:[]}).code, 'empty-cond-set');
    console.log('Installed npm package acceptance passed:', engineVersion);
  `], { cwd: dir, stdio: 'inherit' });
} finally { await rm(dir, { recursive: true, force: true }); }
