// Generates ../utility/tables.go from Tailwind CSS itself: the static
// utilities, the static variants, the property order that sorts utilities
// and the CSS color names. The Go side builds the rest from the theme.
//
//   node gen-utility.mjs

import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { createRequire } from "node:module";
import { __unstable__loadDesignSystem } from "tailwindcss";

const require = createRequire(import.meta.url);
const pkg = require("tailwindcss/package.json");
const theme = readFileSync(require.resolve("tailwindcss/theme.css"), "utf8");
const ds = await __unstable__loadDesignSystem(theme + "\n@tailwind utilities;", {});

// Static utilities: one line each, name|nodes, where nodes are separated by
// ";" and are "property:value", "@--name,initial-value,syntax" for an
// @property rule, or "&selector{nodes}" for a nested rule.
const statics = [];
const skipped = [];
for (const name of ds.utilities.keys("static")) {
  const [u, ...more] = ds.utilities.get(name).filter((u) => u.kind === "static");
  const nodes = u.compileFn({ kind: "static", root: name, variants: [], important: false, raw: name });
  let ok = more.length === 0;
  const enc = (n) => {
    switch (n.kind) {
      case "declaration":
        if (n.value?.includes(";")) ok = false;
        return n.value === undefined ? null : `${n.property}:${n.value}`;
      case "at-root":
        return n.nodes
          .map((p) => {
            const get = (k) => p.nodes.find((d) => d.property === k)?.value;
            if (p.name !== "@property" || get("inherits") !== "false") ok = false;
            const syntax = get("syntax");
            return `@${p.params},${get("initial-value") ?? ""},${syntax === '"*"' ? "" : syntax.slice(1, -1)}`;
          })
          .join(";");
      case "rule":
        return `&${n.selector}{${n.nodes.map(enc).filter((x) => x !== null).join(";")}}`;
      default:
        ok = false;
        return "";
    }
  };
  const s = nodes.map(enc).filter((x) => x !== null).join(";");
  if (!ok || /[|\n]/.test(s)) skipped.push(name);
  else statics.push(`${name}|${s}`);
}

// Static variants: one line each, name|order|branches|declarations|properties.
// A variant replaces the utility's nodes with one branch each, separated by
// tabs: rules ("R selector", with & for the utility's selector) and at-rules
// ("A prelude") nested in each other, separated by " => ", around the
// utility's nodes. The declarations and @property rules (as CSS, separated
// by tabs) go before the utility's nodes.
const variants = [];
let breakpointOrder = 0;
const themeBreakpoints = new Set([...theme.matchAll(/--breakpoint-([\w-]+):/g)].map((m) => m[1]));
for (const [name, v] of ds.variants.variants) {
  if (v.kind !== "static") continue;
  if (themeBreakpoints.has(name)) {
    breakpointOrder = v.order;
    continue;
  }
  const marker = { kind: "declaration", property: "--marker", value: "1" };
  const node = { kind: "rule", selector: "&", nodes: [marker] };
  if (v.applyFn(node, { kind: "static", root: name }) === null || node.selector !== "&") throw new Error(`variant ${name}`);
  let inner = null;
  const branches = node.nodes.map((top) => {
    const wrappers = [];
    for (let n = top; ; ) {
      if (n.kind === "rule") wrappers.push(`R ${n.selector}`);
      else if (n.kind === "at-rule") wrappers.push(`A ${n.name}${n.params ? " " + n.params : ""}`);
      else throw new Error(`variant ${name}: ${n.kind}`);
      const i = n.nodes.indexOf(marker);
      if (i >= 0) {
        const before = n.nodes.slice(0, i);
        if (n.nodes.length !== i + 1 || (inner !== null && before.length !== inner.length)) throw new Error(`variant ${name}: nodes after the utility's`);
        inner = before;
        break;
      }
      if (n.nodes.length !== 1) throw new Error(`variant ${name}: branches`);
      n = n.nodes[0];
    }
    return wrappers.join(" => ");
  });
  const decls = [];
  const props = [];
  for (const n of inner) {
    if (n.kind === "declaration") decls.push(`${n.property}:${n.value}`);
    else if (n.kind === "at-root") for (const p of n.nodes) props.push(`${p.name} ${p.params} {${p.nodes.map((d) => `${d.property}: ${d.value};`).join(" ")}}`);
    else throw new Error(`variant ${name}: ${n.kind}`);
  }
  variants.push([name, v.order, branches.join("\t"), decls.join(";"), props.join("\t")].join("|"));
}
variants.push(`@breakpoint|${breakpointOrder}`);

// The property order and the color names are not exported, so they are read
// from the package's bundles.
const distDir = dirname(require.resolve("tailwindcss"));
const dist = (f) => readFileSync(join(distDir, f), "utf8");
const lib = readFileSync(require.resolve("tailwindcss"), "utf8");
const order = JSON.parse(lib.match(/\["container-type","pointer-events"[^\]]*\]/)[0]);
const colorsChunk = readdirSync(distDir).map((f) => dist(f)).find((s) => s.includes('new Set(["black","silver"'));
const colors = [...new Set(JSON.parse(colorsChunk.match(/new Set\((\["black","silver"[^\]]*\])\)/)[1]))];

const goString = (lines) => "`" + lines.join("\n") + "`";
const go = `// Code generated by compare/js/gen-utility.mjs from tailwindcss ${pkg.version}; DO NOT EDIT.

package utility

const version = ${JSON.stringify(pkg.version)}

// staticTable lists the static utilities: name|nodes, where nodes are
// separated by ; and are property:value, @--name,initial-value,syntax for an
// @property rule, or &selector{nodes} for a nested rule.
const staticTable = ${goString(statics)}

// variantTable lists the static variants:
// name|order|branches|declarations|properties. Branches are separated by
// tabs; each is rules (R selector) and at-rules (A prelude) nested in each
// other, separated by " => ". Declarations are separated by ; and @property
// rules by tabs. The order of the breakpoint variants, which come from the
// theme, is @breakpoint's.
const variantTable = ${goString(variants)}

// propertyOrder sorts utilities by the properties they set.
const propertyOrder = ${goString(order)}

// namedColors are CSS's color keywords.
const namedColors = ${goString(colors)}
`;
writeFileSync(new URL("../utility/tables.go", import.meta.url), go);
if (skipped.length) console.error("skipped static utilities:", skipped.join(" "));
