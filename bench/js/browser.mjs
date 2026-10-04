// The browser counterpart of run.mjs: runs the suite for ?impl= and leaves
// the results in globalThis.benchResults (compare.mjs reads them through
// Playwright) as well as on the page.

import { DEFAULT_OPTS, run } from "./harness.mjs";
import { LOADERS } from "./loaders.mjs";
import { SUITE } from "./suite.mjs";

const host = {
  loadScript: (url) =>
    new Promise((resolve, reject) => {
      const s = document.createElement("script");
      s.src = url.href;
      s.onload = resolve;
      s.onerror = () => reject(new Error(`loading ${url}`));
      document.head.append(s);
    }),
  readBytes: async (url) => {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`fetching ${url}: ${res.status}`);
    return res.arrayBuffer();
  },
};

const params = new URLSearchParams(location.search);
const impl = params.get("impl");
const only = params.getAll("kernel");
const opts = { ...DEFAULT_OPTS };
for (const k of Object.keys(opts)) if (params.has(k)) opts[k] = Number(params.get(k));
const suite = only.length ? SUITE.filter((k) => only.includes(k.name)) : SUITE;

const out = document.getElementById("out");
const results = [];
const report = (r) => {
  results.push(r);
  out.textContent += JSON.stringify(r) + "\n";
};

try {
  if (!LOADERS[impl]) throw new Error(`unknown impl ${impl}`);
  const t = performance.now();
  const fns = await LOADERS[impl](new URL("../out/", import.meta.url), host);
  report({ startup: performance.now() - t });
  await run(fns, suite, opts, report);
  globalThis.benchResults = { results };
} catch (e) {
  globalThis.benchResults = { results, error: String(e && e.stack || e) };
}
