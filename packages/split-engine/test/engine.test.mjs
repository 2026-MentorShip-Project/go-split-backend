import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { initEngine, splitDetail, validateRule, engineVersion } from '../dist/index.js';

const members = [{ id: 3 }, { id: 2 }, { id: 1 }];
const preview = (detail, rules = []) => splitDetail({ detail, members, rules });

test('explicit initialization, failed fetch retry, and one shared runtime', async () => {
  assert.throws(() => preview({ amount: 101 }), /initEngine/);
  assert.throws(() => validateRule({ groups: [], cond_tags: [] }), /initEngine/);
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

test('rule validation mirrors the backend and reports stable codes', () => {
  const accepted = validateRule({ groups: [{ conds: ['vegan'], mode: 'exclude' }], cond_tags: ['vegan'] });
  assert.equal(accepted.ok, true);
  assert.deepEqual(accepted.groups, [{ conds: ['vegan'], mode: 'exclude', weight: 0 }]);
  assert.deepEqual(accepted.rest, { mode: 'weight', weight: 1 });

  // Defaults land the same way a save would apply them.
  const defaulted = validateRule({ groups: [{ conds: ['kid'], mode: 'weight' }], cond_tags: ['kid'] });
  assert.deepEqual(defaulted.groups, [{ conds: ['kid'], mode: 'weight', weight: 1 }]);
  assert.deepEqual(validateRule({ groups: [{ conds: ['kid'], mode: 'weight', weight: 0 }], cond_tags: ['kid'] }).groups,
    [{ conds: ['kid'], mode: 'exclude', weight: 0 }]);

  const refusals = [
    [{ groups: [{ conds: [], mode: 'exclude' }], cond_tags: [] }, 'empty-cond-set'],
    [{ groups: [{ conds: ['nope'], mode: 'exclude' }], cond_tags: ['kid'] }, 'unknown-cond'],
    [{ groups: [{ conds: ['kid'], mode: 'weight', weight: 2.55 }], cond_tags: ['kid'] }, 'invalid-weight'],
    [{ groups: [{ conds: ['kid'], mode: 'weight', weight: 101 }], cond_tags: ['kid'] }, 'invalid-weight'],
    [{ groups: [{ conds: ['kid'], mode: 'skip' }], cond_tags: ['kid'] }, 'invalid-mode'],
    [{ groups: [{ conds: ['kid'], mode: 'exclude' }, { conds: ['kid'], mode: 'exclude' }], cond_tags: ['kid'] }, 'duplicate-cond-set'],
    [{ groups: 'not an array', cond_tags: [] }, 'invalid-groups'],
  ];
  for (const [input, code] of refusals) {
    const verdict = validateRule(input);
    assert.equal(verdict.ok, false, JSON.stringify(input));
    assert.equal(verdict.code, code);
    assert.equal(typeof verdict.detail, 'string');
  }

  // Only a malformed call throws; a refused rule is a result.
  assert.throws(() => validateRule(undefined), /invalid JSON input/);
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
