# @go-split/engine

The Go-Split allocation engine compiled from Go to WebAssembly. The frontend needs neither Go nor backend source code. This previews one unsaved expense detail; saving and settlement still use the backend API.

## Browser use

```js
import { initEngine, splitDetail, engineVersion } from '@go-split/engine';

// Once on the client, e.g. React useEffect / Vue onMounted.
await initEngine();
const result = splitDetail({
  detail: { amount: 101, item_tag: '', custom_amounts: {} },
  members: [
    { id: 1, cond_tags: [] },
    { id: 2, cond_tags: [] },
    { id: 3, cond_tags: [] }
  ],
  rules: [],
  split_order: [1, 2, 3]
});
// validity: 'ok'; share amounts: 34, 34, 33.
```

Initialization downloads the binary and starts the bundled Go runtime once. Concurrent calls share initialization. `splitDetail()` is synchronous afterward and makes no network requests. Failed downloads can be retried; incompatible binary versions fail explicitly. Importing the wrapper is safe during SSR; initialize on the client.

The default binary URL uses `new URL('./engine.wasm', import.meta.url)`. Your bundler must emit this asset and preserve runtime side effects. Vite also supports explicit asset URLs:

```js
import wasmURL from '@go-split/engine/engine.wasm?url';
import { initEngine, splitDetail } from '@go-split/engine';
await initEngine({ wasmURL });
```

For frameworks requiring explicit public assets, copy the exported `engine.wasm` into the frontend's public directory and pass `initEngine({ wasmURL: '/assets/engine.wasm' })`. The wrapper imports the matching JavaScript runtime; no HTML script tag is needed. Version/cache the binary with its JS package. Serve through HTTP(S). Node/custom loaders may supply `initEngine({ wasmBytes })` with a Uint8Array or ArrayBuffer. Node tests use Node 24.

This wrapper runs on the calling thread, without creating a Worker. Each context supports one engine version. Deployment CSP must permit WebAssembly execution and loading the asset.

## Inputs and results

Use validated R1 inputs, integer NT dollars, safe JavaScript integer IDs/amounts, unique member IDs, and valid weights. The backend still authoritatively validates saves. Map API member `tags` to `cond_tags`, and detail `tag` to `item_tag`. Preserve member order / `split_order` for deterministic remainders.

`custom_amounts` holds fixed overrides, never calculated results. Omitted/null `manual_member_ids` uses all members. Pass the item's payer as `payer_id`: a detail nobody shares, such as an empty `manual_member_ids` list or a rule that excludes everyone, goes entirely to the payer with trace kind `payer-absorbs`, so it nets to zero. Rules contain ordered AND groups and optional rest; modes are `weight` and `exclude`.

Results include shares, exclusions, traces and validity: `ok`, `no-participant` (nobody shares the detail and `payer_id` is not a member), `custom-overflow`, or `custom-mismatch`. TypeScript declarations are included.

## Checking a rule before saving

`validateRule()` applies the same checks the backend applies on save, so a draft rule can be corrected without a round trip.

```js
import { initEngine, validateRule } from '@go-split/engine';

await initEngine();
validateRule({
  groups: [{ conds: ['vegan'], mode: 'exclude' }],
  rest: { mode: 'weight', weight: 1 },
  cond_tags: ['vegan', 'kid']
});
// { ok: true, groups: [{ conds: ['vegan'], mode: 'exclude', weight: 0 }], rest: { mode: 'weight', weight: 1 } }
```

A refused rule is a result, not an exception: `{ ok: false, code, detail }`. Codes are stable — `invalid-groups`, `invalid-rest`, `empty-cond-set`, `duplicate-cond-set`, `unknown-cond`, `invalid-mode`, `invalid-weight` — so callers can show their own wording instead of the English `detail`. Only a malformed call throws.

On success the returned `groups` and `rest` are normalized exactly as the backend would store them: an omitted weight becomes 1, weight 0 becomes `exclude`, and an omitted `rest` becomes weight 1. Pass the event's condition tag catalog as `cond_tags`; a rule may only reference tags it contains. Checks needing the database — whether the item tag exists, whether expenses already use it, whether a rule for that tag exists — remain server-side, so saving can still be refused.

## Build in the backend repository

```sh
npm run build --prefix packages/split-engine
npm test --prefix packages/split-engine
cd packages/split-engine
mkdir -p packed
npm pack --pack-destination packed
node scripts/test-package.mjs packed/*.tgz
```

No npm dependencies. Building needs Go and Node 24. Packing builds/tests automatically; installing the published package does not run a Go build. The WASM binary and JS runtime come from the same Go installation. The runtime's license is included as `dist/GO-LICENSE`. Project code remains UNLICENSED, matching the repository's lack of a selected open-source license.

See the backend's `docs/npm-engine-release.md` for release and authentication setup.
