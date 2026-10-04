// SVG bar charts of compare.mjs results, written next to the results
// (results/charts/) by compare.mjs and report.mjs and shown in the READMEs.
// Bars are horizontal on a logarithmic axis (the implementations differ by
// orders of magnitude), one group per runtime or kernel and one bar per
// implementation; a dashed line marks native Go. Text follows the viewer's
// light or dark color scheme.

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const COLORS = { js: "#8c959f", goesm: "#2f81f7", gopherjs: "#d29922", gowasm: "#a371f7", tinygo: "#3fb950" };

const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

function fmt(v) {
  if (v >= 100) return v.toFixed(0);
  if (v >= 10) return v.toFixed(1);
  if (v >= 1) return v.toFixed(2);
  return v.toPrecision(2);
}

// textWidth estimates the width of s at 12px: CJK characters are about
// twice as wide as Latin ones.
const textWidth = (s) => [...s].reduce((w, c) => w + (c.charCodeAt(0) > 0x2e80 ? 12 : 7), 0);

// ticks returns the 1-2-5 steps from lo to hi.
function ticks(lo, hi) {
  const out = [];
  for (let e = Math.floor(Math.log10(lo)); e <= Math.ceil(Math.log10(hi)); e++) {
    for (const m of [1, 2, 5]) {
      const v = m * 10 ** e;
      if (v >= lo * 0.999 && v <= hi * 1.001) out.push(v);
    }
  }
  return out;
}

// barChart draws groups ({label, bars: [{id, value, text}]}) with a legend of
// impls ({id, label}); ref ({value, label}) is the dashed reference line.
export function barChart({ title, subtitle, groups, impls, ref, unit = "" }) {
  const W = 760, left = 150, right = 80, plotW = W - left - right;
  const barH = 13, barGap = 3, groupGap = 16;
  const top = 78;
  const values = groups.flatMap((g) => g.bars.map((b) => b.value)).filter((v) => v > 0);
  if (ref) values.push(ref.value);
  if (values.length === 0) values.push(1);
  // The axis runs between 1-2-5 steps around the values, with room on the
  // right for the longest bar's label.
  const steps = ticks(Math.min(...values) / 10, Math.max(...values) * 20);
  const lo = steps.filter((v) => v <= Math.min(...values) * 0.9).pop();
  let hi = steps.find((v) => v >= Math.max(...values) * 1.3);
  if (hi / lo < 10) hi = lo * 10;
  const x = (v) => left + (plotW * Math.log10(v / lo)) / Math.log10(hi / lo);
  const groupH = (g) => g.bars.length * (barH + barGap) - barGap;
  const plotH = groups.reduce((s, g) => s + groupH(g) + groupGap, 0) - groupGap;
  const H = top + plotH + 36;

  const o = [];
  o.push(`<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" font-family="-apple-system, 'Segoe UI', Helvetica, Arial, 'Hiragino Sans', 'Noto Sans CJK JP', 'Yu Gothic', sans-serif" font-size="12">`);
  o.push(`<style>.t{fill:#1f2328}.m{fill:#59636e}.g{stroke:#d1d9e0}.r{stroke:#1f2328}@media (prefers-color-scheme:dark){.t{fill:#f0f6fc}.m{fill:#9198a1}.g{stroke:#3d444d}.r{stroke:#f0f6fc}}</style>`);
  o.push(`<text class="t" x="0" y="18" font-size="15" font-weight="600">${esc(title)}</text>`);
  if (subtitle) o.push(`<text class="m" x="0" y="37">${esc(subtitle)}</text>`);
  // Legend.
  let lx = 0;
  for (const i of impls) {
    o.push(`<rect x="${lx}" y="50" width="11" height="11" rx="2" fill="${COLORS[i.id] ?? "#888"}"/>`);
    o.push(`<text class="t" x="${lx + 16}" y="60">${esc(i.label)}</text>`);
    lx += 16 + textWidth(i.label) + 18;
  }
  // Grid and axis labels.
  for (const v of ticks(lo, hi)) {
    o.push(`<line class="g" x1="${x(v).toFixed(1)}" y1="${top - 4}" x2="${x(v).toFixed(1)}" y2="${top + plotH + 4}"/>`);
    o.push(`<text class="m" x="${x(v).toFixed(1)}" y="${top + plotH + 18}" text-anchor="middle">${fmt(v)}${unit}</text>`);
  }
  // Bars.
  let y = top;
  for (const g of groups) {
    const h = groupH(g);
    o.push(`<text class="t" x="${left - 10}" y="${(y + h / 2 + 4).toFixed(1)}" text-anchor="end">${esc(g.label)}</text>`);
    for (const b of g.bars) {
      if (b.value > 0) {
        const w = Math.max(1, x(b.value) - left);
        o.push(`<rect x="${left}" y="${y}" width="${w.toFixed(1)}" height="${barH}" rx="2" fill="${COLORS[b.id] ?? "#888"}"><title>${esc(b.title ?? b.text)}</title></rect>`);
        o.push(`<text class="t" x="${(left + w + 4).toFixed(1)}" y="${y + barH - 2}" font-size="11">${esc(b.text)}</text>`);
      } else {
        o.push(`<text class="m" x="${left + 4}" y="${y + barH - 2}" font-size="11">—</text>`);
      }
      y += barH + barGap;
    }
    y += groupGap - barGap;
  }
  if (ref) {
    const rx = x(ref.value).toFixed(1);
    o.push(`<line class="r" x1="${rx}" y1="${top - 6}" x2="${rx}" y2="${top + plotH + 4}" stroke-dasharray="4 3" stroke-width="1.2"/>`);
    o.push(`<text class="t" x="${rx}" y="${top + plotH + 32}" text-anchor="middle" font-size="11">${esc(ref.label)}</text>`);
  }
  o.push("</svg>");
  return o.join("\n") + "\n";
}

// chartFiles returns the charts of data for lang as {file name: SVG}, using
// the table helpers of report.mjs (passed in to avoid an import cycle).
export function chartFiles(data, lang, h) {
  const sfx = lang === "ja" ? ".ja" : "";
  const L = lang === "ja"
    ? {
        slowdown: "ネイティブ Go に対する遅さ（幾何平均、対数軸、短いほど速い）",
        total: "全カーネルを 1 回ずつ実行した合計時間（ms、対数軸、短いほど速い）",
        kernels: (r) => `カーネルごとのネイティブ Go 比（${r}、対数軸、短いほど速い）`,
        native: "ネイティブ Go",
        partial: "* はないカーネルを除いた値",
        noAdd: "Add はネイティブ Go の値がないので除く",
      }
    : {
        slowdown: "Slowdown vs native Go (geometric mean, log scale, shorter is faster)",
        total: "Total ms to run every kernel once (log scale, shorter is faster)",
        kernels: (r) => `Time vs native Go per kernel (${r}, log scale, shorter is faster)`,
        native: "native Go",
        partial: "* without the kernels the implementation lacks",
        noAdd: "Add has no native Go time and is left out",
      };
  const impls = data.impls.map((i) => ({ id: i.id, label: h.implLabel(i, lang) }));
  const runtimes = Object.keys(data.runs);
  const files = {};

  files[`slowdown${sfx}.svg`] = barChart({
    title: L.slowdown,
    impls,
    ref: { value: 1, label: L.native },
    unit: "×",
    groups: runtimes.map((r) => {
      const sd = h.slowdowns(data, r);
      return { label: h.runtimeLabel(data, r, lang), bars: impls.map((i) => ({ id: i.id, value: sd[i.id], text: sd[i.id] ? `${sd[i.id].toFixed(1)}×` : "" })) };
    }),
  });

  const nt = h.total(data, data.native);
  files[`total${sfx}.svg`] = barChart({
    title: L.total,
    subtitle: L.partial,
    impls,
    ref: nt && { value: nt.sum, label: `${L.native} ${fmt(nt.sum)}${nt.partial ? "*" : ""}` },
    groups: runtimes.map((r) => ({
      label: h.runtimeLabel(data, r, lang),
      bars: impls.map((i) => {
        const t = h.total(data, data.runs[r][i.id]);
        return { id: i.id, value: t?.sum, text: t ? `${fmt(t.sum)}${t.partial ? "*" : ""}` : "" };
      }),
    })),
  });

  for (const r of runtimes) {
    files[`kernels-${r}${sfx}.svg`] = barChart({
      title: L.kernels(h.runtimeLabel(data, r, lang)),
      subtitle: L.noAdd,
      impls,
      ref: { value: 1, label: L.native },
      unit: "×",
      groups: data.suite
        .filter((k) => data.native.kernels[k.name])
        .map((k) => ({
          label: k.name,
          bars: impls.map((i) => {
            const t = h.time(data, data.runs[r][i.id], k);
            const n = h.perCall(k, data.native.kernels[k.name].median);
            const v = t === undefined ? undefined : t / n;
            return { id: i.id, value: v, text: v === undefined ? "" : `${fmt(v)}×`, title: v === undefined ? "" : `${h.implLabel(i, lang)}: ${fmt(t)} ${k.calls ? "ns" : "ms"}` };
          }),
        })),
    });
  }
  return files;
}

// writeCharts writes the charts of data in both languages to dir.
export function writeCharts(dir, data, h) {
  mkdirSync(dir, { recursive: true });
  for (const lang of ["en", "ja"]) {
    for (const [name, svg] of Object.entries(chartFiles(data, lang, h))) writeFileSync(join(dir, name), svg);
  }
}
