// Turns the JSON lines of run.mjs (one file per runtime) into the tables of
// ../README.md and ../README.ja.md, between their <!-- compare:start -->
// and <!-- compare:end --> markers. Sizes are Node.js's zlib's.
//
//   node report.mjs ../results/node.jsonl ../results/bun.jsonl

import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { LIBS } from "./libs.mjs";

const dir = fileURLToPath(new URL("..", import.meta.url));
const runs = process.argv.slice(2).map((f) => readFileSync(f, "utf8").trim().split("\n").filter(Boolean).map((l) => JSON.parse(l)));
const runtimes = runs.map((r) => r[0].runtime);

const kb = (n) => (n / 1024).toFixed(1);
const ms = (n) => (n < 1 ? n.toFixed(2) : n < 10 ? n.toFixed(1) : n.toFixed(0));
const ratio = (a, b) => (a / b).toFixed(2) + "×";

function table(lang) {
  const h = lang === "ja"
    ? ["比較対象", "Go パッケージ", "goesm gzip", "JS gzip", "比"]
    : ["Library", "Go package", "goesm gzip", "JS gzip", "ratio"];
  const sizeRows = [`| ${h.join(" | ")} |`, `|${h.map(() => " --- ").join("|")}|`];
  for (const lib of LIBS) {
    const r = runs[0].find((x) => x.name === lib.name);
    if (!r) continue;
    sizeRows.push(`| ${lib.name} | \`${lib.pkg}\` | ${kb(r.go.size.gzip)} KiB | ${kb(r.js.size.gzip)} KiB | ${ratio(r.go.size.gzip, r.js.size.gzip)} |`);
  }
  const t = lang === "ja" ? ["比較対象", ...runtimes.flatMap((rt) => [`${rt} goesm`, `${rt} JS`, "比"])] : ["Library", ...runtimes.flatMap((rt) => [`${rt} goesm`, `${rt} JS`, "ratio"])];
  const timeRows = [`| ${t.join(" | ")} |`, `|${t.map(() => " --- ").join("|")}|`];
  for (const lib of LIBS) {
    const cells = [lib.name];
    for (const run of runs) {
      const r = run.find((x) => x.name === lib.name);
      cells.push(r ? `${ms(r.go.ms)} ms` : "", r ? `${ms(r.js.ms)} ms` : "", r ? ratio(r.go.ms, r.js.ms) : "");
    }
    timeRows.push(`| ${cells.join(" | ")} |`);
  }
  return sizeRows.join("\n") + "\n\n" + timeRows.join("\n");
}

for (const [file, lang] of [["README.md", "en"], ["README.ja.md", "ja"]]) {
  const p = dir + file;
  const s = readFileSync(p, "utf8");
  const out = s.replace(/<!-- compare:start -->[\s\S]*<!-- compare:end -->/, `<!-- compare:start -->\n${table(lang)}\n<!-- compare:end -->`);
  writeFileSync(p, out);
}
