// Loading each implementation's build output (see build.sh) and returning
// its kernels as an object of plain JS functions. The host supplies how to
// run a classic script and read a file, so the same code serves Node.js,
// Bun (run.mjs) and browsers (browser.mjs). base is the URL of out/.

import { waitFor } from "./harness.mjs";

async function runWasm(base, dir, host) {
  await host.loadScript(new URL(`${dir}/wasm_exec.js`, base));
  const go = new globalThis.Go();
  const bytes = await host.readBytes(new URL(`${dir}/bench.wasm`, base));
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance); // resolves when main exits, which it does not
  const fns = await waitFor(() => globalThis.goBench, "goBench");
  return { fns, instance };
}

// wasmExports returns the calling kernels of jsmain/export_wasm.go as JS
// functions: strings go through linear memory as UTF-8.
function wasmExports(instance) {
  const x = instance.exports;
  const memory = x.mem ?? x.memory; // Go's name, TinyGo's
  const enc = new TextEncoder();
  const dec = new TextDecoder();
  const str = (f) => (s) => {
    const max = s.length * 3;
    const p = x.in(max);
    const { written } = enc.encodeInto(s, new Uint8Array(memory.buffer, p, max));
    const n = f(written);
    const q = x.out(); // before reading memory.buffer, which a call may replace
    return dec.decode(new Uint8Array(memory.buffer, q, n));
  };
  return { Add: x.add, Upper: str(x.upper), Handle: str(x.handle) };
}

export const LOADERS = {
  // The hand-written JavaScript reference.
  async js(base) {
    return { ...(await import(new URL("../js/handwritten.mjs", base).href)) };
  },
  // goesm build output: an ES module exporting the package's functions.
  // Go strings are byte strings; the runtime converts JS strings in and out.
  async goesm(base) {
    const m = await import(new URL("goesm/kernels.js", base).href);
    const rt = m.$runtime;
    const str = (f) => (s) => rt.toJSString(f(rt.fromJSString(s)));
    return { ...m, Upper: str(m.Upper), Handle: str(m.Handle) };
  },
  // GopherJS: a script whose main sets globalThis.goBench.
  async gopherjs(base, host) {
    await host.loadScript(new URL("gopherjs/bench.js", base));
    return await waitFor(() => globalThis.goBench, "goBench");
  },
  // Go's js/wasm port: wasm_exec.js and the module. The kernels go through
  // syscall/js; the calling kernels through the wasm exports of
  // jsmain/export_wasm.go.
  async gowasm(base, host) {
    const { fns, instance } = await runWasm(base, "gowasm", host);
    return { ...fns, ...wasmExports(instance) };
  },
  // TinyGo's wasm target: as Go's.
  async tinygo(base, host) {
    const { fns, instance } = await runWasm(base, "tinygo", host);
    return { ...fns, ...wasmExports(instance) };
  },
};
