// Run: node tests/wasm/smoke.cjs engine.wasm /path/to/wasm_exec.js
const fs = require('node:fs');
const assert = require('node:assert/strict');
globalThis.crypto ||= require('node:crypto').webcrypto;
require(process.argv[3]);
(async () => {
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject);
  void go.run(instance);
  const split = (detail, rules = []) => JSON.parse(goSplitDetail(JSON.stringify({
    detail, members: [{ id: 3 }, { id: 2 }, { id: 1 }], rules,
  })));
  const equal = split({ amount: 101 });
  assert.equal(equal.validity, 'ok');
  assert.deepEqual(equal.shares.map(s => [s.member_id, s.amount]), [[3, 34], [2, 34], [1, 33]]);
  const fixed = split({ amount: 101, custom_amounts: { 3: 20 } });
  assert.deepEqual(fixed.shares.map(s => s.amount), [20, 41, 40]);
  assert.equal(split({ amount: 101, manual_member_ids: [] }).validity, 'no-participant');
  assert.equal(split({ amount: 101, custom_amounts: { 3: 120 } }).validity, 'custom-overflow');
  const ruled = split({ amount: 101, item_tag: 'meal' }, [{ item_tag: 'meal', groups: [], rest: { mode: 'weight' } }]);
  assert.deepEqual(ruled.shares.map(s => s.amount), [34, 34, 33]);
  const excluded = split({ amount: 101, item_tag: 'meal' }, [{ item_tag: 'meal', groups: [], rest: { mode: 'weight', weight: 0 } }]);
  assert.equal(excluded.validity, 'no-participant');
  assert.equal(excluded.excluded.length, 3);
  console.log('WASM acceptance checks passed');
  process.exit(0);
})().catch(error => { console.error(error); process.exit(1); });
