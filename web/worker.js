// One Go runtime per Web Worker. The page sends the compiled module once,
// then calls the functions web/wasm/main.go registers on the global scope.
importScripts('wasm_exec.js');

let ready;

self.onmessage = async ({ data }) => {
  if (data.type === 'boot') {
    const go = new Go();
    ready = WebAssembly.instantiate(data.module, go.importObject).then((instance) => {
      go.run(instance); // never resolves: main blocks on select{}
    });
    return;
  }
  const { id, fn, args } = data;
  try {
    await ready;
    self.postMessage({ id, result: self[fn](...args) });
  } catch (err) {
    self.postMessage({ id, error: String(err && err.message || err) });
  }
};
