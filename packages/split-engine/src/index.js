import { engineVersion } from './version.js';
export { engineVersion };

let initialization;
let split;
let runtimeError;

/** Load once before previewing. Importing this module alone is safe during SSR. */
export function initEngine(options = {}) {
  return initialization ??= start(options).catch(error => {
    initialization = undefined;
    throw error;
  });
}

async function start({ wasmURL = new URL('./engine.wasm', import.meta.url), wasmBytes } = {}) {
  if (typeof globalThis.goSplitDetail === 'function') {
    checkVersion();
    split = globalThis.goSplitDetail;
    return;
  }
  await import('./wasm_exec.js');
  let bytes = wasmBytes;
  if (bytes === undefined) {
    const response = await fetch(wasmURL);
    if (!response.ok) throw new Error(`Cannot load split engine: HTTP ${response.status}`);
    bytes = await response.arrayBuffer();
  }
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  // Go's main stays alive to serve previews, so awaiting run would never finish.
  void go.run(instance).then(
    () => { runtimeError = new Error('Split engine stopped'); },
    error => { runtimeError = error; }
  );
  checkVersion();
  if (typeof globalThis.goSplitDetail !== 'function') throw new Error('Split engine did not initialize');
  split = globalThis.goSplitDetail;
}

function checkVersion() {
  if (globalThis.goSplitEngineVersion !== engineVersion) {
    throw new Error(`WASM version mismatch: expected ${engineVersion}, got ${globalThis.goSplitEngineVersion}`);
  }
}

/** Synchronous calculation after initialization. Does not save or settle anything. */
export function splitDetail(input) {
  if (runtimeError) throw runtimeError;
  if (!split) throw new Error('Call and await initEngine() before splitDetail()');
  const result = JSON.parse(split(JSON.stringify(input)));
  if (result.error) throw new Error(result.error);
  return result;
}
