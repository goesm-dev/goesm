<h1 align="center"><img src="docs/assets/goesm.png" alt="goesm" width="480"></h1>

[![CI](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/goesm-dev/goesm.svg)](https://pkg.go.dev/github.com/goesm-dev/goesm)
[![Go version](https://img.shields.io/github/go-mod/go-version/goesm-dev/goesm)](go.mod)
[![Release](https://img.shields.io/github/v/release/goesm-dev/goesm?include_prereleases&sort=semver)](https://github.com/goesm-dev/goesm/releases)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

[日本語](README.ja.md)

Compile Go packages into native ES modules, emitted as TypeScript. No WebAssembly.

goesm takes ordinary Go packages from an ordinary Go module and turns each one into an ES module that Vite, Rolldown, esbuild, Bun, Node.js or a browser can import directly. Exported Go functions become JavaScript functions, exported types become classes with TypeScript types, and Go's semantics (integer arithmetic, slices, maps, interfaces, goroutines, `defer` / `panic` / `recover`, generics, reflection) are preserved and checked against native Go.

> [!NOTE]
> goesm is experimental. See [Status](#status) for what works today and what does not.

```go
package cart

type Item struct {
	Name     string
	Price    int
	Quantity int
}

func Total(items []Item) int {
	total := 0
	for _, item := range items {
		total += item.Price * item.Quantity
	}
	return total
}

func Discount(total, percent int) int {
	return total * (100 - percent) / 100
}
```

```ts
import { Discount, Item, Total, $runtime as rt } from "./goesm-ts/example.com/app/cart.ts";

const items = rt.sliceLit([new Item(rt.fromJSString("apple"), 120, 3), new Item(rt.fromJSString("bread"), 250, 1)]);
Total(items);          // 610
Discount(2408, 15);    // 2046: Go's integer division, not 2046.8
```

## Why goesm

- **Go functions are JavaScript functions.** There is no WebAssembly instance, no `wasm_exec.js`, no asynchronous instantiation and no value marshalling through `syscall/js`: calling `Discount` costs what calling any JS function costs, and numbers, booleans and structs cross the boundary as they are.
- **One ES module per Go package.** The output is a tree of TypeScript modules that import each other with relative `.ts` specifiers, so the host's bundler does tree shaking, code splitting, minification and source maps (back to the `.go` files), and TypeScript sees the Go API's types.
- **Go stays Go.** goesm uses the Go toolchain itself (go/packages, go/types) as the frontend: `go.mod`, `go.work`, `gopls`, `go vet` and `go test` keep working on the same code, and there is no goesm-specific syntax. The standard library is compiled from Go's own source.
- **Checked against native Go.** Every fixture's results are compared with `go run`, and Go's own test suite (`$GOROOT/test`) runs through goesm.

## Status

The proof of concept compiles most of the Go language and a good part of the standard library:

- **Language:** functions and closures, structs, arrays, slices, maps, pointers, interfaces, type switches, generics (including Go 1.27 generic methods), method values, `defer` / `panic` / `recover`, goroutines, channels, `select`, range over integers and functions, `goto`, labeled statements, package initialization order.
- **Standard library**, compiled from Go source: `strings`, `strconv`, `unicode`, `sort`, `slices`, `maps`, `errors`, `math`, `math/bits`, `fmt`, `reflect`, `encoding/json`, `sync`, `time` (on the host's timers), `os` standard streams, and more. A function goesm cannot lower yet becomes a stub that panics if called; `goesm build -v` lists them.
- **Compile-time instrumentation:** `goesm build -toolexec "otelc toolexec"` builds a program instrumented by OpenTelemetry's [otelc](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation), and it emits the spans a native build does, to the console or to an OTLP/HTTP collector ([docs/otelc.md](docs/otelc.md)). `//go:linkname` between packages and `//go:embed` work too.
- **Go's test suite:** 898 of the 961 runnable tests in `$GOROOT/test` produce the output native Go does ([docs/conformance.md](docs/conformance.md)).
- **Use cases:** command-line tools (including cobra), build tools, server-side rendering, HTTP and Connect servers with `http.ListenAndServe` under Node.js, Bun and Deno, the same `http.Handler` as a Cloudflare Workers fetch handler, Connect clients, DOM code, and React, Preact or Next.js apps calling Go. [docs/use-cases.md](docs/use-cases.md) says what is supported where, how each is checked, and what is not supported yet.

Not there yet (details in [ARCHITECTURE.md §11](ARCHITECTURE.md#11-implemented--not-implemented--differences-from-native-go)):

- `int` and `uint` are JS numbers: exact below 2^53, but they do not wrap on 64-bit overflow. `int64` and `uint64` are exact (BigInt).
- There is no JS calling ABI yet: Go strings and slices are runtime objects, converted by hand with the runtime each module re-exports (`rt.fromJSString`, `rt.sliceLit`, `rt.toArray`, ...). Functions that may block return Promises.
- Goroutine-local `recover` state, deadlock detection while the host has pending work, DOM bindings.

## Install

goesm needs the Go toolchain (Go 1.27 or later; an older `go` downloads 1.27 by itself through `GOTOOLCHAIN`). It does not need Node.js or npm: the runtime (`@goesm/runtime`) is embedded in the binary and written into the output.

Add goesm as a tool of your module, so that it is built with the same toolchain as your code:

```sh
go get -tool github.com/goesm-dev/goesm/cmd/goesm@latest
go tool goesm emit-ts ./cart     # goesm-ts/<import path of cart>.ts + goesm-ts/@goesm/runtime/
```

or install it on your `PATH`:

```sh
go install github.com/goesm-dev/goesm/cmd/goesm@latest
```

There are no prebuilt binaries: goesm runs `go` anyway, and building it with your own toolchain keeps its go/types in step with the Go your module uses. While goesm is experimental, releases are prereleases named `v0.0.1-beta.N` ([GitHub Releases](https://github.com/goesm-dev/goesm/releases)); `@latest` resolves to the newest one. `goesm version` prints the goesm version and the Go it was built with.

## Usage

### `goesm emit-ts`: a TypeScript tree for your bundler

Inputs are ordinary Go package patterns in an ordinary Go module:

```sh
cd testdata/example
go run ../../cmd/goesm emit-ts -o goesm-ts ./main   # -o defaults to goesm-ts
```

```text
goesm-ts/
├── example.com/app/main.ts    import * as mathx from "./mathx.ts"
├── example.com/app/mathx.ts   import * as $rt from "../../@goesm/runtime/index.ts"
└── @goesm/runtime/
    ├── index.ts
    └── ...                    the other runtime files
```

The module of Go package `p` is `<dir>/<p>.ts`, standard library packages included (`strings.ts`, `internal/bytealg.ts`), and the runtime is `<dir>/@goesm/runtime/` (a Go import path cannot start with `@`). Modules import each other with relative specifiers ending in `.ts`, so no resolver, plugin or bundler configuration is needed. Import the entry package from your own code:

```ts
// index.ts in a Vite project, or run directly: bun index.ts / node index.ts (Node.js 22.18+)
import { Result } from "./goesm-ts/example.com/app/main.ts";
console.log(Result()); // 3
```

To type-check code that imports the tree with `tsc`, enable `allowImportingTsExtensions`. [docs/example-output.md](docs/example-output.md) shows the generated TypeScript and JavaScript.

### `goesm build`: a bundled ES module

`goesm build` writes the same tree to a temporary directory and bundles it with esbuild's Go API, for when there is no bundler (and for goesm's own tests):

```sh
go run ../../cmd/goesm build ./main            # dist/main.js (+ .js.map pointing at the .go files)
go run ../../cmd/goesm build -minify ./main
go run ../../cmd/goesm build -split ./main     # one ES module per Go package: dist/example.com/app/main.js, ...
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

The output is ES modules with a `.js` extension. Under Node.js, load them from a package whose `package.json` says `"type": "module"`: otherwise Node.js parses each module a second time to detect its format, which for a large bundle adds tens of milliseconds to startup.

### Rebuilds

goesm keeps the TypeScript module of every package it lowers in a cache, `goesm/modules` in the user cache directory. `GOESMCACHE` moves the cache, and `GOESMCACHE=off` disables it. A rebuild lowers only the packages whose module can have changed: the edited packages, the packages that depend on them, and the packages whose whole-program analysis results the edit changed, such as a function of a dependency that now has to await a callback. The output is the same with and without the cache. For the `site` package of goesm.dev, which has 77 packages, `emit-ts` takes 1.2 s without the cache and 0.7 s after editing one package; the Go frontend and the whole-program analysis take the rest.

### Calling Go from JavaScript

- Exported functions and types are exports of the package's module, with their Go types in TypeScript (`Total(items: $rt.S<Item>): number`).
- Numbers and booleans are JS numbers and booleans; `int64` / `uint64` are BigInts. Structs are classes with positional constructors (`new Item(name, price, quantity)`).
- Go strings are byte strings: `rt.fromJSString(s)` in, `rt.toJSString(s)` out. Slices: `rt.sliceLit([...])` in, `rt.toArray(s)` out. Multiple results come back as an array, and an `error` as a Go interface value.
- A function that may block (channel operations, `time.Sleep`, waiting on a mutex) is an `async function` and returns a Promise; the others are synchronous.

`rt` is the runtime, which every module re-exports as `$runtime`. [examples/](examples) has runnable examples (the cart, standard library use, goroutines) with the JavaScript that calls them.

### Serving HTTP

An `http.Handler` (a `ServeMux`, a Connect service, middleware) serves requests on every host, through the host's own server:

```go
// Node.js, Bun and Deno: the host's HTTP server (node:http, Bun.serve, Deno.serve) runs it.
log.Fatal(http.ListenAndServe(":8080", api.Handler()))
```

```ts
// Cloudflare Workers (and Deno.serve, Bun.serve, service workers): a fetch handler.
import { Handler, $runtime as rt } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: rt.fetchHandler(Handler()) };
```

Each request runs in its own goroutine, its body read in full first; the response is sent when the handler returns, or streams from its first flush (Server-Sent Events, Connect's server streaming). Under Workers, `os.Getenv` reads the Worker's text bindings and secrets with the `nodejs_compat` flag. The HTTP client is `fetch`. [docs/use-cases.md](docs/use-cases.md) lists what is supported where.

## Performance

[bench/](bench) runs the same Go kernels compiled by goesm, [GopherJS](https://github.com/gopherjs/gopherjs), Go's own `GOOS=js GOARCH=wasm` port and [TinyGo](https://tinygo.org)'s wasm target under Node.js, Bun and Chromium, checks every result against native Go, and also measures startup time and output size. Native Go and the same kernels written by hand in JavaScript are references. What each kernel does, how the outputs are built and called, and the fairness caveats are in [bench/README.md](bench/README.md). All numbers, per runtime, are in [bench/results/results.md](bench/results/results.md).

How the numbers are measured:

- Every implementation is measured on its prebuilt output. Building and the TypeScript-to-JavaScript step happen beforehand and are in no measured time. Loading the output, including compiling and instantiating wasm, is left out of the kernel times and counted in startup time.
- Native Go runs the same measuring loop in Go, inside a binary built with `go build`.
- Hand-written JS is [bench/js/handwritten.mjs](bench/js/handwritten.mjs), imported as an ES module as it is.
- For goesm, the TypeScript that `goesm build -minify` emits is bundled into one ES module, `kernels.js`, by the esbuild built into goesm, and that module is imported.
- For GopherJS, the script `gopherjs build -m` emits is loaded. For Go wasm and TinyGo wasm, the `.wasm` file and `wasm_exec.js` are loaded.
- Each kernel is called from JavaScript. After a warm-up of at least 300 ms and at least 3 calls, it is timed for at least 10 calls and at least 1000 ms, and the median is the result. A kernel whose calls are slow stops after at least 3 calls once 10 s have passed, even if it has fewer than 10 calls.
- Add, Upper and Handle measure one call of a function from JavaScript. For goesm and wasm, that time includes converting strings between JS and Go, because the caller pays for that conversion.

<!-- bench:start -->
- Intel(R) Xeon(R) Processor @ 2.80GHz (4 threads), linux 6.18.44-fc-v70
- goesm f2910eb, go version go1.27.1 linux/amd64
- GopherJS 1.21.0+go1.21.13
- tinygo version 0.42.0 linux/amd64 (using go version go1.27.1 and LLVM version 22.1.4)
- Node.js v26.10.0, Bun 1.4.2, Chromium 141.0.7390.37
- 2026-10-05; warm-up ≥ 300 ms, then the median of ≥ 10 calls and ≥ 1000 ms per kernel, or of ≥ 3 calls once 10 s have passed for slow kernels

![Slowdown vs native Go](bench/results/charts/slowdown.svg)

![Total time](bench/results/charts/total.svg)

Slowdown against native Go, as the geometric mean of the per-kernel time ratios. Lower is better.

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 0.97× | **1.14×** | 11.37× | 2.78× | 1.73× |
| Bun 1.4.2 | 1.38× | **1.15×** | 10.59× | 3.05× | 1.90× |
| Chromium 141.0.7390.37 | 0.92× | **1.06×** | 9.61× | 3.05× | 1.79× |

Total ms to run every kernel once: the sum of the medians, with the calling kernels' whole loops included. A value marked * leaves out the kernels that implementation lacks. Lower is better.

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 407* | **569** | 15305 | 1884 | 1186 |
| Bun 1.4.2 | 1335* | **538** | 15170 | 1878 | 1338 |
| Chromium 141.0.7390.37 | 367* | **503** | 12336 | 2127 | 1269 |

The kernel on which each implementation is the slowest relative to native Go and to hand-written JS, with the time ratio. The cliff kernels below are included.

| Runtime | | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | Worst kernel vs native Go | 55.0× RSASign | 327× RSASign | **8.12× Upper** | 8.61× Upper |
| Node.js v26.10.0 | Worst kernel vs hand-written JS | 88.3× RSASign | 10672× Add | **21.0× Upper** | 22.3× Upper |
| Bun 1.4.2 | Worst kernel vs native Go | 141× Rand64 | 631× RSASign | **6.13× RSASign** | 12.0× RSASign |
| Bun 1.4.2 | Worst kernel vs hand-written JS | 98.2× RSASign | 3598× Add | **14.0× Upper** | 27.9× Pull |
| Chromium 141.0.7390.37 | Worst kernel vs native Go | 57.5× RSASign | 277× RSASign | **11.6× Upper** | 11.8× Upper |
| Chromium 141.0.7390.37 | Worst kernel vs hand-written JS | 129× RSASign | 8009× Add | **31.4× Upper** | 31.9× Upper |

Median ms per call under Node.js v26.10.0. The rows marked ns/call give the time of one call from JS in ns. Lower is better.

| Kernel | Exercises | Native Go | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fib | recursive calls, int arithmetic | 4.80 | 12.5 | 13.1 | 13.1 | 18.3 | **5.43** |
| Sieve | []bool, tight loops | 10.8 | 13.7 | 17.5 | 115 | 18.5 | **12.6** |
| Mandelbrot | float64 loops | 16.5 | 16.1 | 16.0 | 16.0 | 16.6 | **15.0** |
| NBody | float64 struct fields via pointers | 11.1 | 15.1 | 18.6 | 1210 | 15.1 | **11.8** |
| FNV32 | uint32 multiply and xor | 12.0 | 11.9 | 12.0 | 28.8 | 22.5 | **11.4** |
| FNV64 | uint64 multiply and xor | 11.5 | 57.6 | 21.4 | 457 | 23.6 | **11.2** |
| BinaryTrees | allocation, GC | 90.3 | 58.3 | **76.1** | 91.9 | 407 | 210 |
| Interfaces | interface method calls | 43.1 | 15.1 | **31.8** | 56.8 | 121 | 42.3 |
| MapInt | map[int]int insert, lookup, delete | 63.9 | 43.6 | 80.7 | **79.2** | 131 | 184 |
| MapString | map[string]int counting | 12.5 | 28.7 | **24.4** | 201 | 37.3 | 84.9 |
| Strings | strings.Builder, strconv, Split, Join | 12.2 | 19.9 | 21.8 | 803 | 42.8 | **17.8** |
| Sort | sort.Ints, sort.Strings | 40.8 | 70.0 | 70.8 | 225 | 142 | **53.4** |
| JSON | encoding/json Marshal + Unmarshal | 13.3 | 5.11 | **5.98** | 900 | 35.8 | 33.4 |
| Sprintf | fmt.Sprintf | 24.4 | 11.6 | **16.5** | 2020 | 77.3 | 58.1 |
| Channels | goroutines, unbuffered channels | 125 | — | 91.0 | 512 | 330 | **64.5** |
| Add, ns/call | calls from JS: two numbers in, one out | — | 0.59 | **0.59** | 6314 | 7.02 | 2.14 |
| Upper, ns/call | calls from JS: strings.ToUpper, a string in and out | 186 | 72.0 | **198** | 21159 | 1513 | 1604 |
| Handle, ns/call | calls from JS: a JSON request handler, a string in and out | 4852 | 2010 | **3095** | 582772 | 29347 | 20886 |
| **Total ms, every kernel once** | | 560* | 407* | **569** | 15305 | 1884 | 1186 |
| **Geometric mean vs native Go** | | 1.00× | 0.97× | **1.14×** | 11.37× | 2.78× | 1.73× |

The cliff kernels, in ms under Node.js v26.10.0: ways of writing Go that a compiler to JS can make far slower than native Go or than the JS one would write. The geometric means and totals above leave them out.

| Kernel | Exercises | Native Go | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Parallel | CPU work split over 4 goroutines | 8.72 | 42.5 | 33.1 | 60.3 | 47.6 | **12.2** |
| Rand64 | uint64 arithmetic on a struct field | 2.80 | 24.0 | 134 | 177 | 3.63 | **2.25** |
| MaybeBlocking | interface calls of which another implementation blocks | 1.63 | 0.97 | 4.90 | 2.60 | 2.62 | **0.47** |
| Pull | iter.Pull | 4.42 | 1.17 | 70.7 | — | **12.9** | 23.6 |
| RSASign | crypto/rsa 2048-bit signatures, math/big | 6.09 | 3.80 | 335 | 1992 | **32.7** | 51.3 |

Startup in ms, from starting to load the output to the first callable function.

| Runtime | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Node.js | 220 | 217 | 82.0 | **64.8** |
| Bun | 496 | 474 | 115 | **50.1** |
| Chromium | 221 | 196 | 145 | **60.7** |

Output size, covering all kernels and the standard library they use.

| | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Files | `kernels.js` | `bench.js` | `bench.wasm` + `wasm_exec.js` | `bench.wasm` + `wasm_exec.js` |
| Raw | **1221 KiB** | 1633 KiB | 5070 KiB | 1338 KiB |
| gzip -9 | 349 KiB | **331 KiB** | 1399 KiB | 504 KiB |
| brotli -11 | 277 KiB | **242 KiB** | 1028 KiB | 363 KiB |
<!-- bench:end -->

What these numbers say about goesm today:

- **Total time: goesm is the fastest of the four on every runtime.** Running every kernel once takes goesm 0.50–0.57 s, TinyGo 1.19–1.34 s, Go wasm 1.88–2.13 s and GopherJS 12.3–15.3 s. Hand-written JS takes 0.37–1.34 s without Channels. In geometric mean against native Go, goesm takes 1.06–1.15×, hand-written JS 0.92–1.38×, TinyGo 1.73–1.90×, Go wasm 2.78–3.05× and GopherJS 9.61–11.37×. Under Bun, goesm's geometric mean is below hand-written JS's. Before optimization started, goesm's was 4.2–4.6×. This run used a different machine from the previous one, so its times are not comparable with earlier versions of this table.
- **The geometric mean is what is left after slower and faster kernels cancel out.** Under Node.js, hand-written JS takes 2.6× native Go's time on Fib, 5.0× on FNV64 and 2.3× on MapString. It takes only 0.38× on JSON, 0.48× on Sprintf and 0.39× on Upper, because the JS engine's built-ins for JSON and strings are faster than Go's standard library. goesm shows the same pattern. It takes 0.93–2.7× native Go's time on the numeric kernels, and less time than native Go on Sprintf, JSON, BinaryTrees, Interfaces and the JSON handler.
- **Crossing from JavaScript costs less than in wasm.** A goesm function is a JS function, so a call of `Add` takes 0.59–0.88 ns, as in hand-written JS. Through plain wasm exports, TinyGo takes 2.1–4.9 ns and Go wasm 6.8–9.7 ns. Through `syscall/js`, a Go wasm call takes microseconds. GopherJS takes 3.7–6.3 µs per call. With a string in and out, a call of `Upper` takes goesm 198 ns, wasm 1.1–2.2 µs and hand-written JS 69–81 ns. A call of the JSON request handler takes goesm 2.6–3.1 µs, TinyGo 21–36 µs, Go wasm 27–29 µs and hand-written JS 1.5–2.2 µs.
- **Some kernels have caught up with hand-written JS, and some still lag.** For each kernel, goesm emits code that moves toward the faster of hand-written JS and native Go. For example, a uint64 local used in a loop is held as two 32-bit integers, and `json.Marshal` of a struct goes through an encoder generated for its type. FNV64 takes 1.8–2.3× native Go's time, faster than the 3.4–78× of hand-written JS, which uses BigInt. Under Node.js, goesm matches hand-written JS within 5% or beats it on Fib, Mandelbrot, FNV32, FNV64, MapString and Sort. The largest gaps are Upper at 2.5–2.9×, Interfaces at 1.3–2.1×, the JSON handler at 1.2–1.9×, JSON at 1.2–1.6× and MapInt at up to 1.9× under Node.js.
- **The cliff kernels show where Go and JavaScript differ.** Each cliff kernel is something one of the two does fast and the other may not. goesm is slowest on RSASign, where it takes 55–68× native Go's time and 88–129× WebCrypto's. math/big multiplies 32-bit words through float64 arithmetic, and BigInt is not used because its arithmetic does not run in constant time. Rand64 keeps a uint64 in a struct field, which stays a BigInt, so it takes 45–141× native Go's time; under Bun this matches hand-written JS with BigInt. Pull takes 13–17× native Go's time and 40–95× a JS generator's, because iter.Pull runs its iterator on a goroutine. Parallel cannot use more than one thread in JavaScript and takes about as long as hand-written JS, which runs the work sequentially. MaybeBlocking, where an interface has an implementation that blocks, takes 2.0–3.6× native Go's time; until the analysis was fixed in this release, it took about 13× longer. The uint64 field and iter.Pull cliffs are planned work; RSA and Parallel follow from what JavaScript provides.
- **Startup is slower than wasm, and size is close to GopherJS.** The kernels now include crypto/rsa and math/big for RSASign. goesm's output starts in 220–496 ms, against 196–474 ms for GopherJS, 82–145 ms for Go wasm and 50–65 ms for TinyGo. With brotli, goesm's output is 277 KiB, GopherJS's 242 KiB, TinyGo's 363 KiB and Go wasm's 1028 KiB.
- **What remains is value representation.** In Interfaces, each struct stored in an interface takes one extra box. In Upper and the JSON handler, receiving a JS string as a Go byte string takes a scan for non-ASCII characters on each call. In JSON, decoding into Go structs costs more than hand-written JS's `JSON.parse` into plain objects.

## How it works

```text
.go ──► go/packages + go/types ──► goesm: Go semantics → TypeScript ──► your bundler / runtime ──► ES modules
         (the Go toolchain)                                              (Vite, Rolldown, esbuild, Bun, Node.js)
```

goesm uses the Go toolchain as the authority on the source language and modern JS tooling as the authority on the output. It does not replace the Go parser, type system or modules, and it does not implement a bundler: it lowers type-checked Go to TypeScript, plus a small runtime written in TypeScript. Some choices, all described in [ARCHITECTURE.md](ARCHITECTURE.md):

- A Go package is the unit of compilation, not a Go program: a goesm project does not need `package main`, and the JS application decides where execution starts.
- Goroutines are cooperative. A whole-program analysis finds the functions that may block; those become `async` functions with `await` at their blocking points, and everything else stays plain synchronous JavaScript.
- Standard library packages are compiled from Go's own source. The few tied to the gc runtime (`runtime`, `reflect`, `internal/reflectlite`, `sync`, `syscall/js`) are replaced by goesm's own Go source, and functions without a Go body are implemented in the runtime.
- Each Go type has a runtime type descriptor, which interfaces, generics (type dictionaries), maps with struct keys and reflection use.

[docs/gopherjs-comparison.md](docs/gopherjs-comparison.md) compares the design with GopherJS'.

### Goals and non-goals

goesm aims to keep the Go development experience as it is (`go.mod`, `go.work`, `go fmt`, `gopls`, `go test`, `go vet`, `golangci-lint`, `govulncheck`) and to reuse the JavaScript ecosystem's tools at the other end.

It is not a Go-inspired language, a WebAssembly runtime, a package manager, a replacement for Vite or Rolldown, or a framework. Framework integration belongs in separate adapters: a Vue SFC with `<script setup lang="go">`, for example, can hand its Go to goesm while Vue keeps the template layer.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md): the design, value representation, goroutines, the standard library, what is implemented and the differences from native Go
- [docs/use-cases.md](docs/use-cases.md): what is supported on Node.js, the edge and in browsers, and what is not yet
- [docs/concurrency.md](docs/concurrency.md): how goroutines run on one JavaScript thread, data races and memory safety compared with Go
- [docs/example-output.md](docs/example-output.md): generated TypeScript and JavaScript
- [docs/conformance.md](docs/conformance.md): running Go's own test suite through goesm
- [docs/otelc.md](docs/otelc.md): OpenTelemetry compile-time instrumentation (otelc) with goesm
- [docs/gopherjs-comparison.md](docs/gopherjs-comparison.md): how goesm differs from GopherJS
- [bench/README.md](bench/README.md): the benchmark
- [examples/](examples): runnable examples
- [CONTRIBUTING.md](CONTRIBUTING.md): development setup, principles and what a pull request needs
- [docs/releasing.md](docs/releasing.md): how releases are made

## Development

```sh
mise install           # Go, Node.js and Bun at the versions pinned in mise.toml (CI uses the same)
npm ci --prefix test   # tsc, oxlint and workerd for TestTSC / TestOxlint / TestUseCaseEdge (optional locally; required in CI)
go test ./...          # needs Go 1.27+ and Node.js 22.18+; Bun is optional
```

`TestGolden` runs every parameterless exported function of the fixtures under native Go and in the goesm-built ESM and requires equal results; `TestExamples`, `TestJS`, `TestTSC` (strict type-checking of the emitted TypeScript), `TestOxlint` and the opt-in `TestGoConformance` cover the rest. [CONTRIBUTING.md](CONTRIBUTING.md) describes each test and what a pull request needs.

## License

goesm is released under the [BSD 3-Clause License](LICENSE). The output of `emit-ts` and `build` includes the Go standard library packages your code uses, compiled from Go's source, which are under [Go's own BSD-style license](https://go.dev/LICENSE).
