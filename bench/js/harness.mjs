// The measuring loop, shared by the Node.js / Bun runner (run.mjs) and the
// browser page (browser.html). It mirrors native/main.go: warm up for at
// least 3 calls and opts.warmup ms, then time single calls until at least
// opts.samples calls and opts.time ms are measured, and report the median.

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
  while ((times.length < opts.samples || total < opts.time) && times.length < 1000) {
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
  for (const { name, arg } of suite) {
    let call;
    if (name === "Add") {
      const add = fns.Add;
      call = () => {
        let s = 0;
        for (let i = 0; i < arg; i++) s = add(s, i) & 0xffff;
        return s;
      };
    } else {
      const f = fns[name];
      if (typeof f !== "function") {
        report({ name, missing: true });
        continue;
      }
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
