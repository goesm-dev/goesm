// The comparisons: for each JavaScript library, the Go package that does
// the same with goesm (../<pkg>), the JavaScript version that uses the
// library (impl/<impl>.mjs, impl defaulting to pkg), and the workload both
// are timed on.

import { readFileSync } from "node:fs";
import { create, toBinary } from "@bufbuild/protobuf";
import { ListTasksResponseSchema, TaskSchema } from "./gen/task/v1/task_pb.js";

// fold mixes a value into a checksum, the same way for both sides.
function fold(acc, v) {
  const s = typeof v === "string" ? v : JSON.stringify(v);
  for (let i = 0; i < s.length; i++) acc = (acc * 31 + s.charCodeAt(i)) % 1000000007;
  return acc;
}

// ---- luxon -------------------------------------------------------------

const offsets = ["Z", "+09:00", "-05:00", "+05:30", "-03:30"];
const pad = (n) => String(n).padStart(2, "0");
const isoInputs = Array.from({ length: 200 }, (_, i) => {
  const y = 2020 + (i % 9);
  const mo = 1 + ((i * 7) % 12);
  const d = 1 + ((i * 13) % 28);
  return `${y}-${pad(mo)}-${pad(d)}T${pad((i * 5) % 24)}:${pad((i * 11) % 60)}:${pad((i * 17) % 60)}${offsets[i % offsets.length]}`;
});

function datetime(m) {
  const out = [];
  for (let i = 0; i < isoInputs.length; i++) {
    const iso = isoInputs[i];
    out.push(m.Shift(iso, (i % 61) - 30), m.Label(iso), m.DaysBetween(iso, isoInputs[(i + 1) % isoInputs.length]));
  }
  for (let mo = 1; mo <= 12; mo++) out.push(...m.MonthGrid(2026, mo));
  return out;
}

// ---- neverthrow --------------------------------------------------------

const orderLines = Array.from({ length: 500 }, (_, i) => {
  const sku = `SK-${(i * 37) % 1000}`;
  switch (i % 10) {
    case 3:
      return `sku=${sku};qty=${i % 7}x;price=${i * 13}`; // not a number
    case 5:
      return `sku=${sku};qty=${100 + i};price=${i * 13}`; // out of range
    case 7:
      return `sku=${sku};price=${i * 13}`; // missing qty
    case 9:
      return `sku=${sku};qty=2;price`; // malformed
    default:
      return ` sku = ${sku} ; qty=${1 + (i % 9)};price=${(i * 131) % 50000}`;
  }
});

// ---- connect-es --------------------------------------------------------

// The server is a fetch stand-in that answers ListTasks from responses
// encoded up front, so that both clients are timed doing the same work:
// encoding the request, the Connect protocol, decoding the response.
const owners = Array.from({ length: 20 }, (_, i) => `user-${i}`);
const responses = new Map();
function taskResponse(owner, limit) {
  const tasks = [];
  for (let i = 0; i < limit; i++) {
    tasks.push(
      create(TaskSchema, {
        id: `${owner}-task-${i}`,
        title: `Task ${i} for ${owner}: write the ${["docs", "tests", "release notes", "benchmarks"][i % 4]}`,
        done: i % 3 === 0,
        dueUnix: BigInt(1790000000 + i * 86400),
        tags: ["go", "esm", "ts"].slice(0, i % 4),
      }),
    );
  }
  return toBinary(ListTasksResponseSchema, create(ListTasksResponseSchema, { tasks }));
}
const latin1 = (b) => {
  let s = "";
  for (let i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
  return s;
};
function installFetch() {
  globalThis.fetch = async (input, init = {}) => {
    const req = input instanceof Request ? input : new Request(input, init);
    const body = new Uint8Array(await req.arrayBuffer());
    // The request is a ListTasksRequest: owner (field 1), limit (field 2).
    const key = latin1(body);
    let res = responses.get(key);
    if (!res) {
      const ownerLen = body[1];
      const owner = new TextDecoder().decode(body.subarray(2, 2 + ownerLen));
      const limit = body[2 + ownerLen + 1];
      res = taskResponse(owner, limit);
      responses.set(key, res);
    }
    return new Response(res, { status: 200, headers: { "content-type": "application/proto" } });
  };
}

async function rpc(m) {
  const out = [];
  for (let i = 0; i < 50; i++) out.push(await m.List("https://api.example.com", owners[i % owners.length], 10 + (i % 3) * 10));
  return out;
}

// ---- React ---------------------------------------------------------------

const products = Array.from({ length: 100 }, (_, i) => ({
  id: 1000 + i,
  name: `Item ${i} <${["mug", "tee", "cap", "pen"][i % 4]}> & co`,
  price: (i * 7919) % 200000,
  tags: ["new", "sale", "eco", "gift"].slice(0, i % 5),
  soldOut: i % 6 === 0,
}));

// ---- VitePress, Astro -------------------------------------------------------

// The document is one of goesm's own, read from docs/ as the build left it.
const doc = readFileSync(new URL("../../docs/concurrency.md", import.meta.url), "utf8");

// The renderers spell some characters differently in HTML: markdown-it and
// goldmark escape quotes and > in text, rehype does not. normHTML undoes
// those escapes, so the results compare equal when the documents are.
const normHTML = (html) => html.replace(/&quot;|&#34;/g, '"').replace(/&#39;|&#x27;/g, "'").replace(/&gt;/g, ">").replace(/\n+/g, "\n").trim();

// ---- Tailwind CSS ---------------------------------------------------------

// The class names of a landing page, as Tailwind's scanner finds them, two
// of them unknown, and the theme Tailwind ships.
const page = readFileSync(new URL("page.html", import.meta.url), "utf8");
const classes = [...new Set([...page.matchAll(/class="([^"]*)"/g)].flatMap((m) => m[1].split(/\s+/)).filter(Boolean))];
const twTheme = readFileSync(new URL("node_modules/tailwindcss/theme.css", import.meta.url), "utf8");

export const LIBS = [
  { name: "luxon", pkg: "datetime", lib: "luxon 3.7.2", run: datetime },
  { name: "neverthrow", pkg: "result", lib: "neverthrow 8.2.0", run: (m) => [m.Check(orderLines)] },
  { name: "connect-es", pkg: "rpc", lib: "@connectrpc/connect(-web) 2.2.0, @bufbuild/protobuf 2.16.0", run: rpc, setup: installFetch },
  { name: "react", pkg: "render", lib: "react, react-dom 19.3.0", run: (m) => [m.Render("Shop <All> & more", products)] },
  { name: "vitepress", pkg: "markdown", impl: "markdown-it", lib: "markdown-it 15.0.2", run: (m) => [m.Render(doc)], norm: normHTML },
  // Astro renders Markdown when it builds a site, under Node.js: the
  // browser build of remark would decode entities through the DOM.
  { name: "astro", pkg: "markdown", impl: "remark", platform: "node", lib: "unified 11.0.5, remark-parse 11.0.0, remark-rehype 11.1.2, rehype-stringify 10.0.1", run: (m) => [m.Render(doc)], norm: normHTML },
  { name: "vue", pkg: "reactive", lib: "@vue/reactivity 3.5.43", run: (m) => [m.Bench(50, 20, 2000)] },
  { name: "tailwind", pkg: "utility", impl: "tailwind", lib: "tailwindcss 4.3.3", run: async (m) => [await m.Build(twTheme, classes)] },
];

for (const l of LIBS) l.impl ??= l.pkg;

// checksum runs lib's workload on module m and folds its results, as
// lib.norm normalizes them.
export async function checksum(lib, m) {
  const out = await lib.run(m);
  return out.map((v) => (lib.norm ? lib.norm(v) : v)).reduce(fold, 0);
}
