import { LIBS } from "./libs.mjs";
const side = process.argv[2] ?? "go";
const lib = LIBS.find((l) => l.name === "connect-es (light)");
lib.setup();
const m = await import(side === "go" ? process.env.GOFILE ?? "../out/go/light/rpc.js" : "../out/js/rpc.js");
for (let i = 0; i < 200; i++) await lib.run(m);
const t = performance.now();
for (let i = 0; i < 1000; i++) await lib.run(m);
console.log(side, (performance.now() - t) / 1000);
