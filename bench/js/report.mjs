// Turns compare.mjs results into Markdown. Also a command, to regenerate the
// report from saved results without measuring again, and to copy the
// summary into the repository's README.md and README.ja.md (between the
// <!-- bench:start --> and <!-- bench:end --> lines):
//
//   node js/report.mjs results/results.json > results/results.md
//   node js/report.mjs results/results.json -readme

import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { IMPLS, SUITE } from "./suite.mjs";

const T = {
  en: {
    runtimes: { node: "Node.js", bun: "Bun", chromium: "Chromium" },
    kernel: "Kernel",
    exercises: "Exercises",
    native: "Native Go",
    perCall: "ns/call",
    geomean: "Geometric mean vs native Go",
    total: "Total ms, every kernel once",
    runtime: "Runtime",
    files: "Files",
    raw: "Raw",
  },
  ja: {
    runtimes: { node: "Node.js", bun: "Bun", chromium: "Chromium" },
    kernel: "カーネル",
    exercises: "対象",
    native: "ネイティブ Go",
    perCall: "ns/回",
    geomean: "ネイティブ Go 比の幾何平均",
    total: "合計 ms（全カーネルを 1 回ずつ）",
    runtime: "ランタイム",
    files: "ファイル",
    raw: "非圧縮",
  },
};

function fmt(v) {
  if (v >= 100) return v.toFixed(0);
  if (v >= 10) return v.toFixed(1);
  if (v >= 1) return v.toFixed(2);
  return v.toPrecision(2);
}

const kb = (n) => `${(n / 1024).toFixed(0)} KiB`;

const geomean = (xs) => Math.exp(xs.reduce((s, x) => s + Math.log(x), 0) / xs.length);

const implLabel = (i, lang) => (lang === "ja" && IMPLS.find((x) => x.id === i.id)?.ja) || i.label;
const what = (k, lang) => {
  const s = SUITE.find((x) => x.name === k.name);
  return (lang === "ja" ? s?.ja : s?.what) ?? k.what;
};
const runtimeLabel = (data, r, lang) => data.env[r] ?? T[lang].runtimes[r];

// valid returns the measurement of kernel k in run r, or undefined if the
// kernel is missing, failed or gave a result different from native Go's.
function valid(data, r, k) {
  const m = r?.kernels?.[k.name];
  if (!m || m.error || m.missing) return undefined;
  const want = data.native.kernels[k.name]?.result;
  if (want !== undefined && m.result !== want) return undefined;
  return m;
}

// perCall converts the median ms of a measurement of k to the time the
// tables show: ms per call, or ns per call for the kernels that measure
// calls from JS.
const perCall = (k, ms) => (k.calls ? (ms * 1e6) / k.arg : ms);

// time returns the comparable time of kernel k in run r (see perCall), or
// undefined (see valid).
function time(data, r, k) {
  const m = valid(data, r, k);
  return m && perCall(k, m.median);
}

// total returns the ms run r takes to run every kernel once (the calling
// kernels' whole loops), and whether some kernel was left out.
function total(data, r) {
  let sum = 0, partial = false, any = false;
  for (const k of data.suite) {
    const m = valid(data, r, k);
    if (m) {
      sum += m.median;
      any = true;
    } else partial = true;
  }
  return any ? { sum, partial } : undefined;
}

function cell(data, r, k) {
  const m = r?.kernels?.[k.name];
  if (!m || m.missing) return "—";
  if (m.error) return "error";
  const want = data.native.kernels[k.name]?.result;
  if (want !== undefined && m.result !== want) return "wrong result";
  return fmt(time(data, r, k));
}

// slowdowns returns, per implementation, the geometric mean over the kernels
// native Go runs (all but Add) of its time divided by native Go's. Hand-written
// JS has no Channels; its mean is over the other kernels.
export function slowdowns(data, runtime) {
  const res = {};
  for (const impl of data.impls) {
    const r = data.runs[runtime][impl.id];
    const ratios = [];
    for (const k of data.suite) {
      const n = data.native.kernels[k.name];
      const t = time(data, r, k);
      if (n && t !== undefined) ratios.push(t / perCall(k, n.median));
    }
    res[impl.id] = ratios.length ? geomean(ratios) : undefined;
  }
  return res;
}

// bold marks the smallest of values (undefined skipped) among the
// non-reference implementations.
function bolder(impls, values) {
  const best = Math.min(...values.filter((v, j) => v !== undefined && !impls[j].reference));
  return (j, text) => (values[j] === best && !impls[j].reference ? `**${text}**` : text);
}

export function timeTable(data, runtime, lang = "en") {
  const t = T[lang];
  const impls = data.impls;
  const rows = [
    `| ${t.kernel} | ${t.exercises} | ${t.native} | ${impls.map((i) => implLabel(i, lang)).join(" | ")} |`,
    `| --- | --- | ---: | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const k of data.suite) {
    const b = bolder(impls, impls.map((i) => time(data, data.runs[runtime][i.id], k)));
    const native = data.native.kernels[k.name];
    const name = k.calls ? `${k.name} (${t.perCall})` : k.name;
    rows.push(`| ${name} | ${what(k, lang)} | ${native ? fmt(perCall(k, native.median)) : "—"} | ${impls.map((i, j) => b(j, cell(data, data.runs[runtime][i.id], k))).join(" | ")} |`);
  }
  const tot = impls.map((i) => total(data, data.runs[runtime][i.id]));
  const bt = bolder(impls, tot.map((x) => (x && !x.partial ? x.sum : undefined)));
  const nt = total(data, data.native);
  rows.push(`| **${t.total}** | | ${nt ? `${fmt(nt.sum)}${nt.partial ? "*" : ""}` : "—"} | ${tot.map((x, j) => (x ? bt(j, `${fmt(x.sum)}${x.partial ? "*" : ""}`) : "—")).join(" | ")} |`);
  const sd = slowdowns(data, runtime);
  const b = bolder(impls, impls.map((i) => sd[i.id]));
  rows.push(`| **${t.geomean}** | | 1× | ${impls.map((i, j) => (sd[i.id] ? b(j, `${sd[i.id].toFixed(1)}×`) : "—")).join(" | ")} |`);
  return rows.join("\n");
}

export function summaryTable(data, lang = "en") {
  const impls = data.impls;
  const rows = [
    `| ${T[lang].runtime} | ${impls.map((i) => implLabel(i, lang)).join(" | ")} |`,
    `| --- | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const runtime of Object.keys(data.runs)) {
    const sd = slowdowns(data, runtime);
    const b = bolder(impls, impls.map((i) => sd[i.id]));
    rows.push(`| ${runtimeLabel(data, runtime, lang)} | ${impls.map((i, j) => (sd[i.id] === undefined ? "—" : b(j, `${sd[i.id].toFixed(1)}×`))).join(" | ")} |`);
  }
  return rows.join("\n");
}

// totalTable shows, per runtime, the ms each implementation takes to run
// every kernel once; * marks a total without the kernels the implementation
// lacks.
export function totalTable(data, lang = "en") {
  const impls = data.impls;
  const rows = [
    `| ${T[lang].runtime} | ${impls.map((i) => implLabel(i, lang)).join(" | ")} |`,
    `| --- | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const runtime of Object.keys(data.runs)) {
    const tot = impls.map((i) => total(data, data.runs[runtime][i.id]));
    const b = bolder(impls, tot.map((x) => (x && !x.partial ? x.sum : undefined)));
    rows.push(`| ${runtimeLabel(data, runtime, lang)} | ${tot.map((x, j) => (x ? b(j, `${fmt(x.sum)}${x.partial ? "*" : ""}`) : "—")).join(" | ")} |`);
  }
  return rows.join("\n");
}

export function startupTable(data, lang = "en") {
  const impls = data.impls.filter((i) => !i.reference);
  const rows = [
    `| ${T[lang].runtime} | ${impls.map((i) => implLabel(i, lang)).join(" | ")} |`,
    `| --- | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const runtime of Object.keys(data.runs)) {
    const s = impls.map((i) => data.runs[runtime][i.id]?.startup);
    const b = bolder(impls, s);
    rows.push(`| ${T[lang].runtimes[runtime]} | ${s.map((v, j) => (v === undefined ? "—" : b(j, fmt(v)))).join(" | ")} |`);
  }
  return rows.join("\n");
}

export function sizeTable(data, lang = "en") {
  const t = T[lang];
  const impls = data.impls.filter((i) => data.sizes[i.id]);
  const rows = [`| | ${t.files} | ${t.raw} | gzip -9 | brotli -11 |`, "| --- | --- | ---: | ---: | ---: |"];
  const b = bolder(impls, impls.map((i) => data.sizes[i.id].brotli));
  impls.forEach((i, j) => {
    const s = data.sizes[i.id];
    rows.push(`| ${implLabel(i, lang)} | ${s.files.map((f) => `\`${f.split("/").pop()}\``).join(" + ")} | ${kb(s.raw)} | ${kb(s.gzip)} | ${b(j, kb(s.brotli))} |`);
  });
  return rows.join("\n");
}

export function envList(data, lang = "en") {
  const e = data.env;
  const o = data.opts;
  const method = lang === "ja"
    ? `${data.date.slice(0, 10)} 計測。${o.warmup} ms 以上ウォームアップしたあと、カーネルごとに ${o.samples} 回以上かつ ${o.time} ms 以上計測した中央値`
    : `${data.date.slice(0, 10)}; warm-up ≥ ${o.warmup} ms, then the median of ≥ ${o.samples} calls and ≥ ${o.time} ms per kernel`;
  return [
    `- ${e.cpu}, ${e.os}`,
    `- goesm ${e.goesm}, ${e.go}`,
    `- ${e.gopherjs}`,
    `- ${e.tinygo}`,
    `- ${Object.keys(data.runs).map((r) => runtimeLabel(data, r, lang)).join(", ")}`,
    `- ${method}`,
  ].join("\n");
}

export function markdown(data) {
  const parts = [
    "# Benchmark results",
    "",
    "Generated by `bench/js/compare.mjs`; see [bench/README.md](../README.md) for what is measured and how. Lower is better everywhere; the fastest of goesm, GopherJS, Go wasm and TinyGo wasm is in bold (native Go and hand-written JS are references).",
    "",
    envList(data),
    "",
    "## Slowdown vs native Go (geometric mean over the kernels)",
    "",
    summaryTable(data),
    "",
    "## Total: ms to run every kernel once",
    "",
    "The sum of the medians, the calling kernels' whole loops included; * marks a total without the kernels the implementation lacks (native Go has no Add, hand-written JS no Channels).",
    "",
    totalTable(data),
    "",
  ];
  for (const runtime of Object.keys(data.runs)) {
    parts.push(`## ${runtimeLabel(data, runtime, "en")}: median ms per call`, "", timeTable(data, runtime), "");
  }
  parts.push("## Startup: ms from loading the output to the first callable function", "", startupTable(data), "");
  parts.push("## Output size", "", sizeTable(data), "");
  return parts.join("\n");
}

// readmeBlock is what README.md / README.ja.md show between their markers.
export function readmeBlock(data, lang) {
  const node = Object.keys(data.runs).includes("node") ? "node" : Object.keys(data.runs)[0];
  const h = lang === "ja"
    ? {
        summary: "ネイティブ Go に対する遅さ（各カーネルの時間の比の幾何平均。小さいほど速い）:",
        total: "全カーネルを 1 回ずつ実行した合計時間（ms、中央値の和。呼び出し系カーネルはループ全体。* はないカーネルを除いた値。小さいほど速い）:",
        times: `${runtimeLabel(data, node, lang)} での 1 回あたりの時間（ms、中央値。小さいほど速い）:`,
        startup: "起動時間（出力を読み込み始めてから関数を呼べるようになるまで、ms）:",
        size: "出力サイズ（カーネル一式と、使っている標準ライブラリ）:",
      }
    : {
        summary: "Slowdown vs native Go (geometric mean of the per-kernel time ratios; lower is better):",
        total: "Total ms to run every kernel once (the sum of the medians, the calling kernels' whole loops included; * leaves out kernels the implementation lacks; lower is better):",
        times: `Median ms per call under ${runtimeLabel(data, node, lang)} (lower is better):`,
        startup: "Startup (ms from starting to load the output to the first callable function):",
        size: "Output size (all kernels and the standard library they use):",
      };
  return [
    envList(data, lang), "",
    h.summary, "", summaryTable(data, lang), "",
    h.total, "", totalTable(data, lang), "",
    h.times, "", timeTable(data, node, lang), "",
    h.startup, "", startupTable(data, lang), "",
    h.size, "", sizeTable(data, lang),
  ].join("\n");
}

function updateReadme(file, block) {
  const s = readFileSync(file, "utf8");
  const start = "<!-- bench:start -->";
  const end = "<!-- bench:end -->";
  const i = s.indexOf(start);
  const j = s.indexOf(end);
  if (i < 0 || j < i) throw new Error(`${file}: no ${start} ... ${end} block`);
  writeFileSync(file, `${s.slice(0, i + start.length)}\n${block}\n${s.slice(j)}`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const data = JSON.parse(readFileSync(process.argv[2], "utf8"));
  if (process.argv[3]?.replace(/^--?/, "") === "readme") {
    const root = new URL("../../", import.meta.url);
    updateReadme(fileURLToPath(new URL("README.md", root)), readmeBlock(data, "en"));
    updateReadme(fileURLToPath(new URL("README.ja.md", root)), readmeBlock(data, "ja"));
  } else {
    process.stdout.write(markdown(data));
  }
}
