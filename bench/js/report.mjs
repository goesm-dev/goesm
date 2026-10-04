// Turns compare.mjs results into Markdown. Also a command, to regenerate the
// report from saved results without measuring again:
//
//   node js/report.mjs results/results.json > results/results.md

import { readFileSync } from "node:fs";

const RUNTIME_LABELS = { node: "Node.js", bun: "Bun", chromium: "Chromium" };

function fmt(v) {
  if (v >= 100) return v.toFixed(0);
  if (v >= 10) return v.toFixed(1);
  if (v >= 1) return v.toFixed(2);
  return v.toPrecision(2);
}

const kb = (n) => `${(n / 1024).toFixed(0)} KiB`;

function geomean(xs) {
  return Math.exp(xs.reduce((s, x) => s + Math.log(x), 0) / xs.length);
}

// time returns the comparable time of kernel k in run r: ms per call, or ns
// per call from JS for Add. It is undefined if the kernel failed or gave a
// result different from native Go.
function time(data, r, k) {
  const m = r?.kernels?.[k.name];
  if (!m || m.error || m.missing) return undefined;
  const want = data.native.kernels[k.name]?.result;
  if (want !== undefined && m.result !== want) return undefined;
  return k.name === "Add" ? (m.median * 1e6) / k.arg : m.median;
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
// of its time divided by native Go's.
export function slowdowns(data, runtime) {
  const res = {};
  for (const impl of data.impls) {
    const r = data.runs[runtime][impl.id];
    const ratios = [];
    for (const k of data.suite) {
      const n = data.native.kernels[k.name];
      const t = time(data, r, k);
      if (n && t !== undefined) ratios.push(t / n.median);
    }
    res[impl.id] = ratios.length ? geomean(ratios) : undefined;
  }
  return res;
}

export function timeTable(data, runtime) {
  const impls = data.impls;
  const rows = [
    `| Kernel | Exercises | Native Go | ${impls.map((i) => i.label).join(" | ")} |`,
    `| --- | --- | ---: | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const k of data.suite) {
    const ts = impls.map((i) => time(data, data.runs[runtime][i.id], k));
    const best = Math.min(...ts.filter((t, j) => t !== undefined && !impls[j].reference));
    const cells = impls.map((i, j) => {
      const c = cell(data, data.runs[runtime][i.id], k);
      return ts[j] === best ? `**${c}**` : c;
    });
    const native = data.native.kernels[k.name];
    const name = k.name === "Add" ? "Add (ns/call)" : k.name;
    rows.push(`| ${name} | ${k.what} | ${native ? fmt(native.median) : "—"} | ${cells.join(" | ")} |`);
  }
  const sd = slowdowns(data, runtime);
  rows.push(`| **Geometric mean vs native Go** | | 1× | ${impls.map((i) => (sd[i.id] ? `${sd[i.id].toFixed(1)}×` : "—")).join(" | ")} |`);
  return rows.join("\n");
}

export function summaryTable(data) {
  const impls = data.impls;
  const rows = [
    `| Runtime | ${impls.map((i) => i.label).join(" | ")} |`,
    `| --- | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const runtime of Object.keys(data.runs)) {
    const sd = slowdowns(data, runtime);
    const best = Math.min(...impls.filter((i) => !i.reference && sd[i.id] !== undefined).map((i) => sd[i.id]));
    rows.push(`| ${data.env[runtime] ?? RUNTIME_LABELS[runtime]} | ${impls.map((i) => {
      if (sd[i.id] === undefined) return "—";
      const c = `${sd[i.id].toFixed(1)}×`;
      return sd[i.id] === best && !i.reference ? `**${c}**` : c;
    }).join(" | ")} |`);
  }
  return rows.join("\n");
}

export function sizeTable(data) {
  const rows = [
    "| | Files | Raw | gzip -9 | brotli -11 |",
    "| --- | --- | ---: | ---: | ---: |",
  ];
  for (const i of data.impls) {
    const s = data.sizes[i.id];
    if (!s) continue;
    rows.push(`| ${i.label} | ${s.files.map((f) => `\`${f.split("/").pop()}\``).join(" + ")} | ${kb(s.raw)} | ${kb(s.gzip)} | ${kb(s.brotli)} |`);
  }
  return rows.join("\n");
}

export function startupTable(data) {
  const impls = data.impls;
  const rows = [
    `| Runtime | ${impls.map((i) => i.label).join(" | ")} |`,
    `| --- | ${impls.map(() => "---:").join(" | ")} |`,
  ];
  for (const runtime of Object.keys(data.runs)) {
    rows.push(`| ${RUNTIME_LABELS[runtime]} | ${impls.map((i) => {
      const s = data.runs[runtime][i.id]?.startup;
      return s === undefined ? "—" : fmt(s);
    }).join(" | ")} |`);
  }
  return rows.join("\n");
}

export function envList(data) {
  const e = data.env;
  return [
    `- ${e.cpu}, ${e.os}`,
    `- goesm ${e.goesm}, ${e.go}`,
    `- ${e.gopherjs}`,
    `- ${e.tinygo}`,
    ...Object.keys(data.runs).map((r) => `- ${e[r] ?? RUNTIME_LABELS[r]}`),
    `- ${data.date.slice(0, 10)}; warm-up ≥ ${data.opts.warmup} ms, then ≥ ${data.opts.samples} calls and ≥ ${data.opts.time} ms per kernel; median`,
  ].join("\n");
}

export function markdown(data) {
  const parts = [
    "# Benchmark results",
    "",
    "Generated by `bench/js/compare.mjs`; see [bench/README.md](../README.md) for what is measured and how.",
    "",
    envList(data),
    "",
    "## Slowdown vs native Go (geometric mean over the kernels; lower is better)",
    "",
    summaryTable(data),
    "",
  ];
  for (const runtime of Object.keys(data.runs)) {
    parts.push(`## ${data.env[runtime] ?? RUNTIME_LABELS[runtime]}: median ms per call`, "", timeTable(data, runtime), "");
  }
  parts.push("## Startup: ms from loading the output to the first callable function", "", startupTable(data), "");
  parts.push("## Output size", "", sizeTable(data), "");
  return parts.join("\n");
}

if (import.meta.url === `file://${process.argv[1]}`) {
  process.stdout.write(markdown(JSON.parse(readFileSync(process.argv[2], "utf8"))));
}
