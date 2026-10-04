# Benchmarks

[日本語](README.ja.md)

This directory compares the JavaScript goesm generates with the other ways of running Go in a JavaScript host:

| | Output | Built with |
| --- | --- | --- |
| **goesm** | an ES module per Go package (bundled here) | `goesm build -minify ./kernels` (this checkout) |
| **GopherJS** | a JavaScript program | `gopherjs build -m ./jsmain` (GopherJS 1.21.0, Go 1.21.13) |
| **Go wasm** | WebAssembly + `wasm_exec.js` | `GOOS=js GOARCH=wasm go build -ldflags=-s ./jsmain` (Go 1.27.1) |
| **TinyGo wasm** | WebAssembly + `wasm_exec.js` | `tinygo build -target=wasm -opt=2 -no-debug ./jsmain` (TinyGo 0.42.0) |

Two references frame the numbers: **native Go** (the same kernels compiled by `go build` for the machine) and **hand-written JS** (the same workloads written by hand in idiomatic JavaScript, [js/handwritten.mjs](js/handwritten.mjs)), which shows what the JS engine itself makes of each workload.

The latest results are in [results/results.md](results/results.md) (raw data: [results/results.json](results/results.json)); the main [README](../README.md#performance) summarizes them.

## What is measured

Every implementation runs the same Go source, package [kernels](kernels):

| Kernel | Size | Exercises |
| --- | ---: | --- |
| Fib | 30 | recursive calls, int arithmetic |
| Sieve | 2,000,000 | `[]bool`, tight loops |
| Mandelbrot | 400×400 | float64 loops |
| NBody | 100,000 steps | float64 struct fields through pointers |
| FNV32 | 8 MB | uint32 multiply and xor |
| FNV64 | 8 MB | uint64 multiply and xor |
| BinaryTrees | depth 14 | allocation, garbage collection |
| Interfaces | 1,000,000 | interface method calls |
| MapInt | 200,000 | `map[int]int` insert, lookup, delete |
| MapString | 500,000 | `map[string]int` counting |
| Strings | 100,000 | `strings.Builder`, `strconv`, `Split`, `Join` |
| Sort | 100,000 | `sort.Ints`, `sort.Strings` |
| JSON | 2,000 records | `encoding/json` Marshal + Unmarshal |
| Sprintf | 50,000 | `fmt.Sprintf` |
| Channels | 100,000 values | goroutines, unbuffered channels, `sync.WaitGroup` |
| Add | 100,000 calls | calls from JS: two numbers in, one out |
| Upper | 100,000 calls | calls from JS: `strings.ToUpper`, a string in and out |
| Handle | 10,000 calls | calls from JS: a JSON request handler (`encoding/json` decode, total, encode), a string in and out |

Each kernel takes a size and returns a checksum. The harness checks every implementation's checksum against native Go's, so a number in the results is the time of a correct computation.

The last three are the shape of a library API: many small calls from JavaScript, each converting its arguments and results at the boundary. They are timed as a JS loop over [`callInputs`](js/suite.mjs) (100 different inputs) and reported per call, so they show what one call costs from JS, work and crossing together. Native Go runs the same loop in Go (`CallChecksum` in [kernels/api.go](kernels/api.go)), the cost of the work alone; it has no Add.

The **total** is the sum of the medians of every kernel (the calling kernels' whole loops): the time an implementation takes to run each kernel once, which weighs the slow kernels the way an application would feel them, where the geometric mean weighs every kernel the same.

- **Timing.** For each kernel the harness ([js/harness.mjs](js/harness.mjs); [native/main.go](native/main.go) for native Go) warms up for at least 3 calls and 300 ms, then times single calls until it has at least 10 and 1 s of them (or at least 3 and 10 s for slow calls), and reports the median.
- **Isolation.** Each implementation runs in its own process (Node.js, Bun) or page (Chromium), one after another.
- **Calling.** goesm's output is an ES module whose exports are the Go functions, so the harness imports `kernels` and calls `Fib(30)` directly. GopherJS, Go wasm and TinyGo build programs rather than packages: [jsmain](jsmain) exposes the kernels with `syscall/js` (`js.FuncOf`) on `globalThis.goBench`, which is how those programs are usually called from JS. A `syscall/js` callback must not block, so for them Channels returns a Promise and runs on its own goroutine; under goesm, Channels is itself an async function.
- **Calling kernels.** Each implementation is called the fastest way it offers. goesm's exports are called directly, with the runtime's `fromJSString` / `toJSString` converting strings (Go strings are byte strings). For Go and TinyGo wasm, a `syscall/js` call costs microseconds (Go) to about a millisecond (TinyGo), so [jsmain/export_wasm.go](jsmain/export_wasm.go) exports Add, Upper and Handle as plain WebAssembly functions (`//go:wasmexport` for Go, `//export` for TinyGo, whose `//go:wasmexport` costs about 0.2 ms per call while `main` blocks) and passes strings as UTF-8 through linear memory with `TextEncoder.encodeInto` and `TextDecoder`. GopherJS goes through `syscall/js`, which for it is a plain JS call.
- **Startup** is the time from starting to load the output (reading or fetching it, compiling, running package initialization and `main`) to the first callable function.
- **Size** is that of the files a page has to load: the bundled module for goesm, the script for GopherJS, the `.wasm` plus `wasm_exec.js` for Go and TinyGo. It includes the parts of the standard library the kernels use (`fmt`, `encoding/json`, `sort`, `strconv`, `strings`, `sync`, `math`).

### Fairness notes

- `int` is 32 bits wide under GopherJS and TinyGo's wasm target and 64 bits wide elsewhere; under goesm it is a JS number. The kernels keep `int` values below 2^31 so that every implementation computes the same thing, and use `uint32` / `uint64` where overflow is part of the algorithm.
- TinyGo is built with `-opt=2` (optimize for speed; its default `-opt=z` optimizes for size). Go wasm is built with its defaults, which have no speed/size switch.
- Native Go runs goroutines on several threads; the others are single-threaded. Handing values over unbuffered channels between threads costs more than switching goroutines on one thread, which is why native Go is not the fastest at Channels.
- Hand-written JS is not Go: it uses typed arrays, `Map`, `JSON.stringify` / `JSON.parse` (implemented natively by the engine) and template strings, and does no bounds or nil checks of its own. It has no Channels.
- [kernels](kernels) stays within the Go 1.21 language and standard library so that GopherJS 1.21 compiles it ([gopherjs.mod](gopherjs.mod) is the module file it builds with).

## Running

The tools are pinned: Go, Node.js and Bun in the repository's [mise.toml](../mise.toml), TinyGo in [this directory's](mise.toml); `build.sh` installs GopherJS with `go install` at a fixed version, and Chromium is driven by `playwright-core` pinned in [package.json](package.json).

```sh
cd bench
mise install                   # Go, Node.js, Bun and TinyGo
npm ci                         # playwright-core, for the Chromium runs
mise exec -- sh build.sh       # builds everything into out/
mise exec -- node js/compare.mjs            # results/results.{json,md}, results/charts/*.svg
node js/report.mjs results/results.json -readme   # copy the summary and charts into ../README(.ja).md
```

`compare.mjs` takes `-runtimes node,bun,chromium`, `-impls goesm,gopherjs,gowasm,tinygo,js`, `-kernels Fib,Sieve` and `-quick` (shorter warm-up and fewer samples). Chromium is found the way Playwright finds it (`PLAYWRIGHT_BROWSERS_PATH`, or `npx playwright-core install chromium`); `CHROMIUM_PATH` overrides it. One implementation can also be run by itself (`node js/run.mjs goesm`, `bun js/run.mjs tinygo Fib`), or in any browser by serving `bench/` and opening `js/browser.html?impl=goesm`.

## int64.mjs

[int64.mjs](int64.mjs) is a separate micro-benchmark of the representations goesm considered for `int64` / `uint64` (BigInt, two uint32 halves, objects, a hybrid); see [ARCHITECTURE.md](../ARCHITECTURE.md#64-bit-integers).
