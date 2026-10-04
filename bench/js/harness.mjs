// The measuring loop, shared by the Node.js / Bun runner (run.mjs) and the
// browser page (browser.html). It mirrors native/main.go: warm up for at
// least 3 calls and opts.warmup ms, then time single calls until at least
// opts.samples calls and opts.time ms are measured (or, for calls slower than
// that allows, 3 calls and 10 × opts.time ms), and report the median.

import { callInputs } from "./suite.mjs";

const now = () => performance.now();

const isThenable = (v) => v !== null && typeof v === "object" && typeof v.then === "function";

async function measure(call, opts) {
  let result;
  let first = 0;
  for (let n = 0, start = now(); n < 3 || now() - start < opts.warmup; n++) {
    const t = now();
    let r = call();
    if (isThenable(r)) r = await r;
    if (n === 0) first = now() - t;
    result = r;
  }
  const times = [];
  let total = 0;
  // Slow calls stop at 3 samples once 10 × opts.time has passed.
  while ((times.length < opts.samples || total < opts.time) && times.length < 1000 && !(times.length >= 3 && total >= 10 * opts.time)) {
    const t = now();
    let r = call();
    if (isThenable(r)) r = await r;
    const d = now() - t;
    if (r !== result) throw new Error(`result changed from ${result} to ${r}`);
    total += d;
    times.push(d);
  }
  times.sort((a, b) => a - b);
  const mid = times.length >> 1;
  const median = times.length % 2 ? times[mid] : (times[mid - 1] + times[mid]) / 2;
  return { result, median, min: times[0], first, samples: times.length };
}

// run measures every kernel of suite with fns (name → function) and calls
// report with each result as it is ready.
export async function run(fns, suite, opts, report) {
  for (const { name, arg, calls } of suite) {
    const f = fns[name];
    if (typeof f !== "function") {
      report({ name, missing: true });
      continue;
    }
    let call;
    if (name === "Add") {
      call = () => {
        let s = 0;
        for (let i = 0; i < arg; i++) s = f(s, i) & 0xffff;
        return s;
      };
    } else if (calls) {
      // As kernels.CallChecksum: fold the length and last code unit of
      // each result (all ASCII, so code units are bytes).
      const inputs = callInputs(name);
      call = () => {
        let acc = 0;
        for (let i = 0; i < arg; i++) {
          const out = f(inputs[i % inputs.length]);
          acc = (acc * 31 + out.length + out.charCodeAt(out.length - 1)) % 1000000007;
        }
        return acc;
      };
    } else {
      call = () => f(arg);
    }
    try {
      report({ name, ...(await measure(call, opts)) });
    } catch (e) {
      report({ name, error: String(e && e.stack || e) });
    }
  }
}

export const DEFAULT_OPTS = { warmup: 300, time: 1000, samples: 10 };

// waitFor resolves with get() once it returns a value, polling on timers so
// that programs whose main runs after a tick get to set it.
export async function waitFor(get, what) {
  for (let i = 0; i < 1000; i++) {
    const v = get();
    if (v) return v;
    await new Promise((r) => setTimeout(r, 1));
  }
  throw new Error(`timed out waiting for ${what}`);
}
