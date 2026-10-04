// Runs the suite for one implementation in this process and prints one JSON
// line per kernel, preceded by a {"startup": ms} line. Works under Node.js
// and Bun; compare.mjs runs it once per implementation and runtime.
//
//   node js/run.mjs goesm [Fib Sieve ...]
//   bun js/run.mjs tinygo

import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { DEFAULT_OPTS, run } from "./harness.mjs";
import { LOADERS } from "./loaders.mjs";
import { SUITE } from "./suite.mjs";

const require = createRequire(import.meta.url);
const host = {
  loadScript: async (url) => void require(fileURLToPath(url)),
  readBytes: (url) => readFile(fileURLToPath(url)),
};

const [impl, ...only] = process.argv.slice(2);
if (!LOADERS[impl]) {
  console.error(`usage: run.mjs ${Object.keys(LOADERS).join("|")} [kernel...]`);
  process.exit(2);
}
const opts = { ...DEFAULT_OPTS };
for (const k of Object.keys(opts)) {
  const v = process.env[`BENCH_${k.toUpperCase()}`];
  if (v) opts[k] = Number(v);
}
const suite = only.length ? SUITE.filter((k) => only.includes(k.name)) : SUITE;

const t = performance.now();
const fns = await LOADERS[impl](new URL("../out/", import.meta.url), host);
console.log(JSON.stringify({ startup: performance.now() - t }));
await run(fns, suite, opts, (r) => console.log(JSON.stringify(r)));
// The wasm programs keep a pending promise (main never returns); exit.
process.exit(0);
