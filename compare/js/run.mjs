// Measures every comparison built by build.mjs: the gzip and brotli size of
// both bundles, and the time of the workload on both under the runtime this
// script runs on (node run.mjs, bun run.mjs). Prints one JSON line per
// comparison. The results of both sides must be the same, or it fails.
//
//   node run.mjs [-quick] [pkg...]

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { checksum, LIBS } from "./libs.mjs";

const out = fileURLToPath(new URL("../out", import.meta.url));
const argv = process.argv.slice(2);
const quick = argv.includes("-quick");
const only = argv.filter((a) => !a.startsWith("-"));
const libs = LIBS.filter((l) => !only.length || only.includes(l.pkg) || only.includes(l.name));

const opts = quick ? { warmup: 50, time: 200, samples: 3 } : { warmup: 500, time: 2000, samples: 10 };
const runtime = typeof Bun !== "undefined" ? `bun ${Bun.version}` : `node ${process.versions.node}`;

function size(file) {
  const b = readFileSync(file);
  return {
    raw: b.length,
    gzip: gzipSync(b, { level: 9 }).length,
    brotli: brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }).length,
  };
}

const now = () => performance.now();

// time returns the median time of one workload run, after a warmup. The
// checksum of the results is left out: it costs both sides the same.
async function time(lib, m) {
  for (let n = 0, start = now(); n < 3 || now() - start < opts.warmup; n++) await lib.run(m);
  const times = [];
  let total = 0;
  while (times.length < opts.samples || total < opts.time) {
    const t = now();
    await lib.run(m);
    const d = now() - t;
    times.push(d);
    total += d;
    if (times.length >= 1000) break;
  }
  times.sort((a, b) => a - b);
  const mid = times.length >> 1;
  return times.length % 2 ? times[mid] : (times[mid - 1] + times[mid]) / 2;
}

for (const lib of libs) {
  lib.setup?.();
  const res = { name: lib.name, pkg: lib.pkg, runtime };
  for (const side of ["go", "js"]) {
    const file = join(out, side, `${side === "go" ? lib.pkg : lib.impl}.js`);
    const t0 = now();
    const m = await import(pathToFileURL(file).href);
    const load = now() - t0;
    res[side] = { size: size(file), load, sum: await checksum(lib, m), ms: await time(lib, m) };
  }
  if (res.go.sum !== res.js.sum) {
    res.error = "the two sides return different results";
    process.exitCode = 1;
  }
  console.log(JSON.stringify(res));
}
// React's browser build leaves a MessageChannel open, which would keep the
// process alive.
process.exit();
