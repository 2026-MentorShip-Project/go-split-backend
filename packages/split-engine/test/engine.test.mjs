import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { initEngine, splitDetail, engineVersion } from '../dist/index.js';

const members = [{ id: 3 }, { id: 2 }, { id: 1 }];
const preview = (detail, rules = []) => splitDetail({ detail, members, rules });

test('explicit initialization, failed fetch retry, and one shared runtime', async () => {
  assert.throws(() => preview({ amount: 101 }), /initEngine/);
  const realFetch = globalThis.fetch;
  let requests = 0;
  try {
    globalThis.fetch = async () => { requests++; return new Response('', { status: 404 }); };
    await assert.rejects(initEngine({ wasmURL: '/missing.wasm' }), /HTTP 404/);
    assert.equal(requests, 1);
    const wasmBytes = await readFile(new URL('../dist/engine.wasm', import.meta.url));
    globalThis.fetch = async url => {
      requests++;
      assert.match(String(url), /dist\/engine\.wasm$/);
      return new Response(wasmBytes);
    };
    const first = initEngine();
    assert.equal(initEngine(), first);
    await first;
    assert.equal(globalThis.goSplitEngineVersion, engineVersion);
    assert.equal(requests, 2);
  } finally { globalThis.fetch = realFetch; }
});

test('actual Go engine preserves allocation, manual overrides and validity', () => {
  assert.deepEqual(preview({ amount: 101 }).shares.map(s => s.amount), [34, 34, 33]);
  assert.deepEqual(preview({ amount: 101, custom_amounts: { 3: 20 } }).shares.map(s => s.amount), [20, 41, 40]);
  assert.equal(preview({ amount: 101, manual_member_ids: [] }).validity, 'no-participant');
  assert.equal(preview({ amount: 101, custom_amounts: { 3: 120 } }).validity, 'custom-overflow');
  assert.equal(preview({ amount: 101, custom_amounts: { 3: 20, 2: 20, 1: 20 } }).validity, 'custom-mismatch');
  assert.equal(preview({ amount: 101, item_tag: 'meal' }, [{ item_tag: 'meal', groups: [], rest: { mode: 'weight', weight: 0 } }]).excluded.length, 3);
  assert.throws(() => preview({ amount: 'invalid' }), /invalid JSON input/);
});

test('SSR import is inert and mixed engine versions fail clearly', () => {
  const url = new URL('../dist/index.js', import.meta.url).href;
  execFileSync(process.execPath, ['--input-type=module', '-e', `
    const {initEngine} = await import(${JSON.stringify(url)});
    if (globalThis.Go !== undefined) throw new Error('runtime executed during import');
    globalThis.goSplitDetail = () => '{}';
    globalThis.goSplitEngineVersion = 'old';
    await (await import('node:assert/strict')).default.rejects(initEngine(), /version mismatch/);
  `]);
});

test('release accepts matching stable tag and rejects accidental releases', () => {
  const script = new URL('../scripts/verify-release.mjs', import.meta.url);
  execFileSync(process.execPath, [script.pathname], { env: { ...process.env, GITHUB_REF_NAME: `engine-v${engineVersion}` } });
  for (const tag of ['main', 'v1.1.0', 'engine-v9.9.9', 'engine-v1.1.0-rc.1']) {
    assert.throws(() => execFileSync(process.execPath, [script.pathname], {
      env: { ...process.env, GITHUB_REF_NAME: tag }, stdio: 'pipe',
    }));
  }
});
