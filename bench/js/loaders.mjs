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

export const LOADERS = {
  // The hand-written JavaScript reference.
  async js(base) {
    return { ...(await import(new URL("../js/handwritten.mjs", base).href)) };
  },
  // goesm build output: an ES module exporting the package's functions.
  async goesm(base) {
    return { ...(await import(new URL("goesm/kernels.js", base).href)) };
  },
  // GopherJS: a script whose main sets globalThis.goBench.
  async gopherjs(base, host) {
    await host.loadScript(new URL("gopherjs/bench.js", base));
    return await waitFor(() => globalThis.goBench, "goBench");
  },
  // Go's js/wasm port: wasm_exec.js and the module, through syscall/js.
  async gowasm(base, host) {
    return (await runWasm(base, "gowasm", host)).fns;
  },
  // TinyGo's wasm target: the kernels through syscall/js as with Go, and
  // Add as a plain wasm export, the usual way for numbers.
  async tinygo(base, host) {
    const { fns, instance } = await runWasm(base, "tinygo", host);
    return { ...fns, Add: instance.exports.add };
  },
};
