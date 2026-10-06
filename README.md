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
import { Discount, Total } from "./goesm-ts/example.com/app/cart.ts";

const items = [{ Name: "りんご", Price: 120, Quantity: 3 }, { Name: "bread", Price: 250, Quantity: 1 }];
Total(items);          // 610
Discount(2408, 15);    // 2046: Go's integer division, not 2046.8
```

## Why goesm

- **Go functions are JavaScript functions.** There is no WebAssembly instance, no `wasm_exec.js`, no asynchronous instantiation and no value marshalling through `syscall/js`: calling `Discount` costs what calling any JS function costs. Numbers and booleans cross the boundary as they are, and strings, arrays and objects are converted to Go's values by the types of the declaration.
- **One ES module per Go package.** The output is a tree of TypeScript modules that import each other with relative `.ts` specifiers, so the host's bundler does tree shaking, code splitting, minification and source maps (back to the `.go` files), and TypeScript sees the Go API's types.
- **Go stays Go.** goesm uses the Go toolchain itself (go/packages, go/types) as the frontend: `go.mod`, `go.work`, `gopls`, `go vet` and `go test` keep working on the same code, and there is no goesm-specific syntax; the one directive, `//goesm:import`, is for code that calls JavaScript. The standard library is compiled from Go's own source.
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
- Goroutine-local `recover` state, and deadlock detection while the host has pending work.

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

- The exported functions of the package you build are exports of its module. They take and return JavaScript values, converted by the Go types: a string is a JS string, a slice an array, a struct a plain object named as `encoding/json` names its fields, and `int64` / `uint64` a BigInt. TypeScript sees those types (`Total(items: Array<{ Name?: string; Price?: number; Quantity?: number }> | null): number`).
- A pointer to a struct type with methods is the Go object itself, whose methods JavaScript calls (`cart.Add(item)`).
- Several results come back as an array. A final `error` result is thrown as a `GoError`, which is the Go error again when it is passed back to Go.
- A function that may block (channel operations, `time.Sleep`, waiting on a mutex) is an `async function` and returns a Promise; the others are synchronous.

[docs/js-exports.md](docs/js-exports.md) has the details. [examples/](examples) has runnable examples (the cart, standard library use, goroutines) with the JavaScript that calls them.

### Calling JavaScript from Go

A function declared without a body imports a function of an ES module, and its Go types say how the values are converted:

```go
//goesm:import "./format.ts" formatPrice
func formatPrice(yen int, currency string) string

//goesm:import "./api.ts" fetchUser await
func fetchUser(id string) (User, error) // a Promise; an exception or rejection is the error
```

Strings, slices, maps, structs (as `encoding/json` names their fields), functions, `js.Value` and `any` cross the boundary, and a call costs a few nanoseconds more than the same call from JavaScript. [docs/js-imports.md](docs/js-imports.md) has the details; Vue components are used through [gosfc](https://github.com/goesm-dev/gosfc).

### Serving HTTP

An `http.Handler` (a `ServeMux`, a Connect service, middleware) serves requests on every host, through the host's own server:

```go
// Node.js, Bun and Deno: the host's HTTP server (node:http, Bun.serve, Deno.serve) runs it.
log.Fatal(http.ListenAndServe(":8080", api.Handler()))
```

```ts
// Cloudflare Workers (and Deno.serve, Bun.serve, service workers): a fetch handler.
import { Handler } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: Handler() }; // an http.Handler result is a fetch handler
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
- Intel(R) Xeon(R) Processor @ 2.10GHz (4 threads), linux 6.18.44-fc-v70
- goesm v0.0.1-beta.1-26-g4bfd0d9, go version go1.27.1 linux/amd64
- GopherJS 1.21.0+go1.21.13
- tinygo version 0.42.0 linux/amd64 (using go version go1.27.1 and LLVM version 22.1.4)
- Node.js v26.10.0, Bun 1.4.2, Chromium 141.0.7390.37
- 2026-10-05; warm-up ≥ 300 ms, then the median of ≥ 10 calls and ≥ 1000 ms per kernel, or of ≥ 3 calls once 10 s have passed for slow kernels

![Slowdown vs native Go](bench/results/charts/slowdown.svg)

![Total time](bench/results/charts/total.svg)

Slowdown against native Go, as the geometric mean of the per-kernel time ratios. Lower is better.

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 0.96× | **1.07×** | 10.42× | 2.50× | 1.83× |
| Bun 1.4.2 | 1.17× | **1.10×** | 8.94× | 2.38× | 1.90× |
| Chromium 141.0.7390.37 | 0.88× | **1.03×** | 8.74× | 2.63× | 1.77× |

Total ms to run every kernel once: the sum of the medians, with the calling kernels' whole loops included. A value marked * leaves out the kernels that implementation lacks. Lower is better.

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 269* | **351** | 8622 | 1039 | 847 |
| Bun 1.4.2 | 870* | **339** | 7267 | 1004 | 863 |
| Chromium 141.0.7390.37 | 239* | **308** | 7108 | 1182 | 775 |

The kernel on which each implementation is the slowest relative to native Go and to hand-written JS, with the time ratio. The cliff kernels below are included.

| Runtime | | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | Worst kernel vs native Go | 54.1× RSASign | 353× RSASign | **5.79× RSASign** | 9.17× Handle |
| Node.js v26.10.0 | Worst kernel vs hand-written JS | 108× RSASign | 4854× Add | **13.4× Upper** | 21.1× Handle |
| Bun 1.4.2 | Worst kernel vs native Go | 98.4× Rand64 | 574× RSASign | **7.49× RSASign** | 10.5× Upper |
| Bun 1.4.2 | Worst kernel vs hand-written JS | 98.8× RSASign | 3224× Add | **18.3× Pull** | 31.3× Pull |
| Chromium 141.0.7390.37 | Worst kernel vs native Go | 68.2× RSASign | 292× RSASign | 10.4× Upper | **8.90× Upper** |
| Chromium 141.0.7390.37 | Worst kernel vs hand-written JS | 107× RSASign | 4626× Add | 38.2× Upper | **32.6× Upper** |

Median ms per call under Node.js v26.10.0. The rows marked ns/call give the time of one call from JS in ns. Lower is better.

| Kernel | Exercises | Native Go | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fib | recursive calls, int arithmetic | 4.06 | 7.65 | 7.85 | 7.71 | 12.2 | **3.90** |
| Sieve | []bool, tight loops | 7.09 | 8.09 | 10.9 | 59.2 | 11.2 | **7.93** |
| Mandelbrot | float64 loops | 9.96 | 11.3 | 11.4 | 11.5 | 11.2 | **10.1** |
| NBody | float64 struct fields via pointers | 8.14 | 7.71 | 8.78 | 676 | 8.72 | **5.99** |
| FNV32 | uint32 multiply and xor | 10.7 | 11.2 | 11.2 | 16.3 | 11.8 | **10.5** |
| FNV64 | uint64 multiply and xor | 11.0 | 41.6 | 14.4 | 272 | 12.9 | **10.8** |
| BinaryTrees | allocation, GC | 52.3 | 43.2 | **50.2** | 60.7 | 214 | 118 |
| Interfaces | interface method calls | 16.8 | 11.5 | **23.4** | 39.2 | 72.8 | 25.2 |
| MapInt | map[int]int insert, lookup, delete | 29.0 | 24.0 | **27.2** | 45.6 | 63.2 | 97.3 |
| MapString | map[string]int counting | 7.42 | 22.7 | **16.9** | 119 | 24.3 | 62.8 |
| Strings | strings.Builder, strconv, Split, Join | 8.60 | 10.3 | 13.4 | 478 | 26.8 | **12.0** |
| Sort | sort.Ints, sort.Strings | 30.3 | 44.4 | 50.1 | 134 | 97.5 | **39.0** |
| JSON | encoding/json Marshal + Unmarshal | 6.78 | 2.47 | **3.38** | 477 | 21.2 | 38.1 |
| Sprintf | fmt.Sprintf | 14.5 | 5.27 | **5.77** | 1306 | 51.8 | 40.7 |
| Channels | goroutines, unbuffered channels | 81.0 | — | 66.2 | 257 | 199 | **23.5** |
| Add, ns/call | calls from JS: two numbers in, one out | — | 0.61 | **0.61** | 2947 | 5.03 | 1.72 |
| Upper, ns/call | calls from JS: strings.ToUpper, a string in and out | 130 | 48.9 | **110** | 13280 | 656 | 667 |
| Handle, ns/call | calls from JS: a JSON request handler, a string in and out | 2992 | 1301 | **1923** | 304050 | 13433 | 27437 |
| **Total ms, every kernel once** | | 341* | 269* | **351** | 8622 | 1039 | 847 |
| **Geometric mean vs native Go** | | 1.00× | 0.96× | **1.07×** | 10.42× | 2.50× | 1.83× |

The cliff kernels, in ms under Node.js v26.10.0: ways of writing Go that a compiler to JS can make far slower than native Go or than the JS one would write. The geometric means and totals above leave them out.

| Kernel | Exercises | Native Go | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Parallel | CPU work split over 4 goroutines | 8.46 | 26.5 | 32.6 | 63.1 | 36.9 | **18.3** |
| Rand64 | uint64 arithmetic on a struct field | 2.09 | 21.7 | 18.1 | 104 | 3.50 | **1.63** |
| MaybeBlocking | interface calls of which another implementation blocks | 2.26 | 0.70 | 1.14 | 1.42 | 1.99 | **0.46** |
| Pull | iter.Pull | 3.98 | 1.11 | **2.23** | — | 8.96 | 10.8 |
| RSASign | crypto/rsa 2048-bit signatures, math/big | 3.89 | 1.94 | 211 | 1372 | **22.5** | 34.1 |

Startup in ms, from starting to load the output to the first callable function.

| Runtime | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Node.js | 141 | 153 | 62.8 | **33.9** |
| Bun | 206 | 189 | 55.3 | **31.8** |
| Chromium | 107 | 155 | 88.1 | **36.5** |

Output size, covering all kernels and the standard library they use.

| | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Files | `kernels.js` | `bench.js` | `bench.wasm` + `wasm_exec.js` | `bench.wasm` + `wasm_exec.js` |
| Raw | **756 KiB** | 1633 KiB | 5070 KiB | 1338 KiB |
| gzip -9 | **220 KiB** | 331 KiB | 1399 KiB | 504 KiB |
| brotli -11 | **175 KiB** | 242 KiB | 1029 KiB | 363 KiB |
<!-- bench:end -->

What these numbers say about goesm today:

- **Total time: goesm is the fastest of the four on every runtime, and under Bun faster than hand-written JS.** Running every kernel once takes goesm 0.31–0.35 s, TinyGo 0.78–0.86 s, Go wasm 1.00–1.18 s and GopherJS 7.1–8.6 s. Hand-written JS takes 0.24–0.87 s without Channels. In geometric mean against native Go, goesm takes 1.03–1.10×, hand-written JS 0.88–1.17×, TinyGo 1.77–1.90×, Go wasm 2.38–2.63× and GopherJS 8.74–10.42×. Under Bun, goesm's geometric mean is below hand-written JS's. Before optimization started, goesm's was 4.2–4.6×. This run used a different machine from the previous one, so its times are not comparable with earlier versions of this table.
- **The geometric mean is what is left after slower and faster kernels cancel out.** Under Node.js, hand-written JS takes 1.9× native Go's time on Fib, 3.8× on FNV64 and 3.1× on MapString. It takes only 0.36× on JSON, 0.36× on Sprintf and 0.38× on Upper, because the JS engine's built-ins for JSON and strings are faster than Go's standard library. goesm shows the same pattern. It takes 0.99–1.93× native Go's time on the numeric kernels. It takes less time than native Go on Sprintf, JSON, Upper and the JSON handler on every runtime, and on BinaryTrees and MapInt under Node.js and Chromium.
- **Crossing from JavaScript costs less than in wasm.** A goesm function is a JS function, so a call of `Add` takes 0.60–0.63 ns, as in hand-written JS. Through plain wasm exports, TinyGo takes 1.7–2.4 ns and Go wasm 5.0–6.3 ns. Through `syscall/js`, a Go wasm call takes microseconds. GopherJS takes 2.0–3.0 µs per call. With a string in and out, a call of `Upper` takes goesm 108–110 ns, wasm 0.66–1.4 µs and hand-written JS 35–54 ns. A call of the JSON request handler takes goesm 1.5–2.1 µs, TinyGo 17–27 µs, Go wasm 13–14 µs and hand-written JS 0.71–1.3 µs.
- **Some kernels have caught up with hand-written JS, and some still lag.** For each kernel, goesm emits code that moves toward the faster of hand-written JS and native Go. For example, a uint64 local used in a loop is held as two 32-bit integers, and `json.Marshal` of a struct goes through an encoder generated for its type. FNV64 takes 1.25–1.33× native Go's time, faster than the 2.4–59× of hand-written JS, which uses BigInt. Under Node.js, goesm matches hand-written JS within 5% or beats it on Fib, Mandelbrot, FNV32, FNV64 and MapString. The largest gaps are Upper at 2.0–3.1×, Interfaces at 1.7–2.0×, the JSON handler at 1.5–2.9×, JSON at 1.4–1.9× and Strings at 1.3–1.4×.
- **The cliff kernels show where Go and JavaScript differ.** Each cliff kernel is something one of the two does fast and the other may not. goesm is slowest on RSASign, where it takes 54–80× native Go's time and 99–108× WebCrypto's. math/big multiplies 32-bit words through float64 arithmetic, and BigInt is not used because its arithmetic does not run in constant time. Rand64 keeps a uint64 in a struct field, which is a BigInt. goesm now works on a local copy of the field through a run of assignments, so Rand64 takes 0.83–1.08× hand-written JS's time with BigInt; that is 4.1–8.7× native Go's time under Node.js and Chromium, and 98× under Bun, whose BigInt arithmetic is slow. Pull now steps a sequence written as a function literal as a JS generator instead of a goroutine, and takes 0.52–0.58× native Go's time and 2.0–5.7× a hand-written JS generator's. Parallel cannot use more than one thread in JavaScript and takes about as long as hand-written JS, which runs the work sequentially. MaybeBlocking, where an interface has an implementation that blocks, takes 0.24–0.50× native Go's time. RSA and Parallel follow from what JavaScript provides.
- **Startup is slower than wasm, and the output is the smallest of the four.** The kernels include crypto/rsa and math/big for RSASign. goesm's output starts in 107–206 ms, against 153–189 ms for GopherJS, 55–88 ms for Go wasm and 32–36 ms for TinyGo. With brotli, goesm's output is 175 KiB, GopherJS's 242 KiB, TinyGo's 363 KiB and Go wasm's 1029 KiB.
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
- [docs/js-imports.md](docs/js-imports.md): calling JavaScript and TypeScript from Go with `//goesm:import`
- [docs/dom.md](docs/dom.md): using the DOM from Go with `syscall/js` and typed DOM libraries
- [docs/example-output.md](docs/example-output.md): generated TypeScript and JavaScript
- [docs/conformance.md](docs/conformance.md): running Go's own test suite through goesm
- [docs/otelc.md](docs/otelc.md): OpenTelemetry compile-time instrumentation (otelc) with goesm
- [docs/gopherjs-comparison.md](docs/gopherjs-comparison.md): how goesm differs from GopherJS
- [bench/README.md](bench/README.md): the benchmark
- [examples/](examples): runnable examples
- [CONTRIBUTING.md](CONTRIBUTING.md): development setup, principles and what a pull request needs
- [CHANGELOG.md](CHANGELOG.md): what changed in each version
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
