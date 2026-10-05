// Builds both sides of every comparison into ../out (see ../README.md):
//
//   out/goesm-ts/<pkg>/   goesm emit-ts ./<pkg>, the TypeScript tree
//   out/go/<pkg>.js       that tree bundled from the package's module
//   out/js/<impl>.js      impl/<impl>.mjs bundled with its library
//
// Both bundles are made the same way, the way a page ships them: esbuild,
// bundled, minified, ES module, for browsers. goesm is built from this
// checkout unless GOESM names a binary.
//
//   node build.mjs [pkg...]

import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { LIBS } from "./libs.mjs";

const dir = fileURLToPath(new URL("..", import.meta.url));
const out = join(dir, "out");
const only = process.argv.slice(2);
const libs = LIBS.filter((l) => !only.length || only.includes(l.pkg) || only.includes(l.name));

let goesm = process.env.GOESM;
if (!goesm) {
  goesm = join(out, "bin", "goesm");
  execFileSync("go", ["build", "-o", goesm, "./cmd/goesm"], { cwd: join(dir, ".."), stdio: "inherit" });
}

const options = {
  bundle: true,
  minify: true,
  format: "esm",
  platform: "browser",
  target: "es2022",
  // What the Go side imports from Node.js only when it runs there.
  external: ["node:*"],
  logLevel: "warning",
};

const built = new Set();
for (const l of libs) {
  if (!built.has(l.pkg)) {
    built.add(l.pkg);
    const tree = join(out, "goesm-ts", l.pkg);
    rmSync(tree, { recursive: true, force: true });
    execFileSync(goesm, ["emit-ts", "-o", tree, `./${l.pkg}`], { cwd: dir, stdio: ["ignore", "ignore", "inherit"] });
    // Like the JavaScript side, the bundle exports only what the workload
    // calls, so that a bundler can drop the rest, as it would in an app.
    const names = [...readFileSync(join(dir, "js/impl", `${l.impl}.mjs`), "utf8").matchAll(/^export (?:async )?function (\w+)/gm)].map((m) => m[1]);
    const entry = join(tree, "entry.ts");
    writeFileSync(entry, `export { ${names.join(", ")} } from "./example.com/compare/${l.pkg}.ts";\n`);
    await build({ ...options, entryPoints: [entry], outfile: join(out, "go", `${l.pkg}.js`) });
  }
  await build({ ...options, platform: l.platform ?? options.platform, entryPoints: [join(dir, "js/impl", `${l.impl}.mjs`)], outfile: join(out, "js", `${l.impl}.js`) });
  console.error(`built ${l.name}`);
}
// ES module packages, as one would ship them: without "type": "module",
// Node.js parses each bundle twice to detect its module format.
for (const side of ["go", "js"]) {
  mkdirSync(join(out, side), { recursive: true });
  writeFileSync(join(out, side, "package.json"), '{"type": "module"}\n');
}
