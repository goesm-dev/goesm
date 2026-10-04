// Runs the whole comparison: native Go, then every implementation under
// every runtime (one process or page each, one after another), measures
// output sizes, and writes the results as JSON plus a Markdown report.
// Build first with build.sh.
//
//   node js/compare.mjs [-runtimes node,bun,chromium] [-impls goesm,...]
//                       [-kernels Fib,...] [-quick] [-o results/results.json]
//
// Chromium is driven with playwright-core (npm ci in bench/); its browser is
// found the way Playwright finds it (PLAYWRIGHT_BROWSERS_PATH, or
// `npx playwright-core install chromium`), or set CHROMIUM_PATH.

import { spawn, execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { cpus, platform, release } from "node:os";
import { dirname, extname, join, normalize, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { DEFAULT_OPTS } from "./harness.mjs";
import { IMPLS, SUITE } from "./suite.mjs";
import { markdown, writeCharts } from "./report.mjs";

const benchDir = fileURLToPath(new URL("..", import.meta.url));
const out = join(benchDir, "out");

const args = { runtimes: "node,bun,chromium", impls: IMPLS.map((i) => i.id).join(","), kernels: "", o: "results/results.json" };
let quick = false;
for (let i = 2; i < process.argv.length; i++) {
  const a = process.argv[i].replace(/^--?/, "");
  if (a === "quick") quick = true;
  else if (a in args) args[a] = process.argv[++i];
  else {
    console.error(`unknown flag ${process.argv[i]}`);
    process.exit(2);
  }
}
const opts = quick ? { warmup: 50, time: 200, samples: 3 } : DEFAULT_OPTS;
const impls = args.impls.split(",");
const runtimes = args.runtimes.split(",");
const kernels = args.kernels ? args.kernels.split(",") : SUITE.map((k) => k.name);
const suite = SUITE.filter((k) => kernels.includes(k.name));

const log = (...a) => console.error(...a);

function sh(cmd, argv, opt = {}) {
  try {
    return execFileSync(cmd, argv, { encoding: "utf8", cwd: benchDir, stdio: ["ignore", "pipe", "ignore"], ...opt }).trim();
  } catch {
    return null;
  }
}

// Runs cmd and returns its stdout lines parsed as JSON, echoing them to
// stderr as they come.
function runJSONLines(cmd, argv, { input, env } = {}) {
  return new Promise((resolve, reject) => {
    // Runtime options from the environment (such as Bun's --smol, a smaller
    // heap that collects more often) would change what is measured.
    const base = { ...process.env };
    delete base.NODE_OPTIONS;
    delete base.BUN_OPTIONS;
    const p = spawn(cmd, argv, { cwd: benchDir, env: { ...base, ...env }, stdio: ["pipe", "pipe", "inherit"] });
    let buf = "";
    const lines = [];
    p.stdout.on("data", (d) => {
      buf += d;
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl);
        buf = buf.slice(nl + 1);
        if (!line.trim()) continue;
        log("   ", line);
        try {
          lines.push(JSON.parse(line));
        } catch {
          // program output that is not ours
        }
      }
    });
    p.on("error", reject);
    p.on("close", (code) => (code === 0 ? resolve(lines) : reject(new Error(`${cmd} ${argv.join(" ")} exited with ${code}`))));
    p.stdin.end(input ?? "");
  });
}

function collect(lines) {
  const run = { kernels: {} };
  for (const l of lines) {
    if ("startup" in l) run.startup = l.startup;
    else run.kernels[l.name] = l;
  }
  return run;
}

// ---- native Go

async function runNative() {
  log("native Go");
  execFileSync("go", ["build", "-o", join(out, "bin", "native"), "./native"], { cwd: benchDir, stdio: "inherit" });
  const ms = (v) => `${v}ms`;
  const lines = await runJSONLines(join(out, "bin", "native"), ["-warmup", ms(opts.warmup), "-time", ms(opts.time), "-samples", String(opts.samples)], {
    // Add measures calls from JS; a Go loop calling Add measures nothing.
    // Upper and Handle run as in JS, called arg times by a Go loop.
    input: JSON.stringify(suite.filter((k) => k.name !== "Add").map(({ name, arg }) => ({ name, arg }))),
  });
  return collect(lines);
}

// ---- Node.js and Bun

async function runProcess(runtime, impl) {
  const env = {};
  for (const [k, v] of Object.entries(opts)) env[`BENCH_${k.toUpperCase()}`] = String(v);
  return collect(await runJSONLines(runtime, ["js/run.mjs", impl, ...suite.map((k) => k.name)], { env }));
}

// ---- Chromium

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".wasm": "application/wasm",
  ".map": "application/json",
};

function serve() {
  const server = createServer((req, res) => {
    const path = normalize(decodeURIComponent(new URL(req.url, "http://x").pathname));
    try {
      if (path.split("/").includes("..")) throw new Error("outside bench/");
      const body = readFileSync(join(benchDir, path));
      res.writeHead(200, {
        "Content-Type": MIME[extname(path)] ?? "application/octet-stream",
        // cross-origin isolation, for performance.now() at full resolution
        "Cross-Origin-Opener-Policy": "same-origin",
        "Cross-Origin-Embedder-Policy": "require-corp",
      });
      res.end(body);
    } catch {
      res.writeHead(404).end();
    }
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve(server)));
}

async function withChromium(f) {
  const { chromium } = await import("playwright-core");
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined });
  const server = await serve();
  try {
    return await f(browser, `http://127.0.0.1:${server.address().port}`);
  } finally {
    server.close();
    await browser.close();
  }
}

async function runPage(browser, origin, impl) {
  const page = await browser.newPage();
  page.on("console", (m) => log("    [console]", m.text()));
  const q = new URLSearchParams({ impl, ...opts });
  for (const k of suite) q.append("kernel", k.name);
  // "commit": the page's module awaits the whole run, which holds back "load".
  await page.goto(`${origin}/js/browser.html?${q}`, { waitUntil: "commit" });
  await page.waitForFunction(() => globalThis.benchResults, null, { timeout: 0, polling: 500 });
  const { results, error } = await page.evaluate(() => globalThis.benchResults);
  await page.close();
  for (const r of results) log("   ", JSON.stringify(r));
  if (error) throw new Error(error);
  return collect(results);
}

// ---- sizes

const FILES = {
  goesm: ["goesm/kernels.js"],
  gopherjs: ["gopherjs/bench.js"],
  gowasm: ["gowasm/bench.wasm", "gowasm/wasm_exec.js"],
  tinygo: ["tinygo/bench.wasm", "tinygo/wasm_exec.js"],
};

function sizes(impl) {
  const s = { files: FILES[impl], raw: 0, gzip: 0, brotli: 0 };
  for (const f of FILES[impl]) {
    const b = readFileSync(join(out, f));
    s.raw += b.length;
    s.gzip += gzipSync(b, { level: 9 }).length;
    s.brotli += brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }).length;
  }
  return s;
}

// ---- main

const data = {
  date: new Date().toISOString(),
  opts,
  env: {
    cpu: `${cpus()[0].model} (${cpus().length} threads)`,
    os: `${platform()} ${release()}`,
    goesm: sh("git", ["describe", "--always", "--dirty"]),
    go: sh("go", ["version"]),
    gopherjs: sh(join(out, "bin", "gopherjs"), ["version"], { env: { ...process.env, GOTOOLCHAIN: "go1.21.13" } }),
    tinygo: sh("tinygo", ["version"]),
  },
  suite,
  impls: IMPLS.filter((i) => impls.includes(i.id)),
  sizes: Object.fromEntries(impls.filter((i) => FILES[i]).map((i) => [i, sizes(i)])),
  native: await runNative(),
  runs: {},
};

// save writes the results so far, so that a failure late in the run keeps
// what was measured.
function save() {
  const file = resolve(benchDir, args.o);
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, JSON.stringify(data, null, 2) + "\n");
  writeFileSync(file.replace(/\.json$/, ".md"), markdown(data));
  writeCharts(join(dirname(file), "charts"), data);
}

for (const runtime of runtimes) {
  data.runs[runtime] = {};
  if (runtime === "chromium") {
    await withChromium(async (browser, origin) => {
      data.env.chromium = `Chromium ${browser.version()}`;
      for (const impl of impls) {
        log(`chromium ${impl}`);
        data.runs.chromium[impl] = await runPage(browser, origin, impl);
        save();
      }
    });
  } else {
    data.env[runtime] = `${runtime === "node" ? "Node.js" : "Bun"} ${sh(runtime, ["--version"])}`;
    for (const impl of impls) {
      log(`${runtime} ${impl}`);
      data.runs[runtime][impl] = await runProcess(runtime, impl);
      save();
    }
  }
}

save();
log(`wrote ${args.o} and ${args.o.replace(/\.json$/, ".md")}`);
