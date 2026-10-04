# goesm

[![CI](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/goesm-dev/goesm/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/goesm-dev/goesm.svg)](https://pkg.go.dev/github.com/goesm-dev/goesm)
[![Go version](https://img.shields.io/github/go-mod/go-version/goesm-dev/goesm)](go.mod)
[![Release](https://img.shields.io/github/v/release/goesm-dev/goesm?include_prereleases&sort=semver)](https://github.com/goesm-dev/goesm/releases)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

[日本語](README.ja.md)

Compile Go packages into native ES modules: plain JavaScript (emitted as TypeScript), no WebAssembly.

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

### Calling Go from JavaScript

- Exported functions and types are exports of the package's module, with their Go types in TypeScript (`Total(items: $rt.S<Item>): number`).
- Numbers and booleans are JS numbers and booleans; `int64` / `uint64` are BigInts. Structs are classes with positional constructors (`new Item(name, price, quantity)`).
- Go strings are byte strings: `rt.fromJSString(s)` in, `rt.toJSString(s)` out. Slices: `rt.sliceLit([...])` in, `rt.toArray(s)` out. Multiple results come back as an array, and an `error` as a Go interface value.
- A function that may block (channel operations, `time.Sleep`, waiting on a mutex) is an `async function` and returns a Promise; the others are synchronous.

`rt` is the runtime, which every module re-exports as `$runtime`. [examples/](examples) has runnable examples (the cart, standard library use, goroutines) with the JavaScript that calls them.

## Performance

[bench/](bench) runs the same Go kernels compiled by goesm, [GopherJS](https://github.com/gopherjs/gopherjs), Go's own `GOOS=js GOARCH=wasm` port and [TinyGo](https://tinygo.org)'s wasm target under Node.js, Bun and Chromium, checks every result against native Go, and also measures startup time and output size. Native Go and the same kernels written by hand in JavaScript are references. What each kernel does, how the outputs are built and called, and the fairness caveats are in [bench/README.md](bench/README.md); all numbers, per runtime, are in [bench/results/results.md](bench/results/results.md).

<!-- bench:start -->
- Intel(R) Xeon(R) Processor @ 2.80GHz (4 threads), linux 6.18.44-fc-v64
- goesm 7be619e, go version go1.27.1 linux/amd64
- GopherJS 1.21.0+go1.21.13
- tinygo version 0.42.0 linux/amd64 (using go version go1.27.1 and LLVM version 22.1.4)
- Node.js v26.10.0, Bun 1.4.2, Chromium 141.0.7390.37
- 2026-10-04; warm-up ≥ 300 ms, then the median of ≥ 10 calls and ≥ 1000 ms per kernel

![Slowdown vs native Go](bench/results/charts/slowdown.svg)

![Total time](bench/results/charts/total.svg)

Slowdown vs native Go (geometric mean of the per-kernel time ratios; lower is better):

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 1.0× | 3.0× | 11.4× | 3.0× | **1.7×** |
| Bun 1.4.2 | 1.2× | 3.8× | 9.5× | 2.7× | **2.0×** |
| Chromium 141.0.7390.37 | 0.9× | 2.6× | 9.1× | 3.4× | **1.8×** |

Total ms to run every kernel once (the sum of the medians, the calling kernels' whole loops included; * leaves out kernels the implementation lacks; lower is better):

| Runtime | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: | ---: |
| Node.js v26.10.0 | 329* | 1534 | 11622 | 1475 | **833** |
| Bun 1.4.2 | 961* | 2806 | 9998 | 1289 | **1036** |
| Chromium 141.0.7390.37 | 282* | 1288 | 9153 | 1684 | **947** |

Median ms per call under Node.js v26.10.0 (lower is better):

| Kernel | Exercises | Native Go | Hand-written JS | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fib | recursive calls, int arithmetic | 4.66 | 10.1 | 9.33 | 9.60 | 14.4 | **3.98** |
| Sieve | []bool, tight loops | 8.21 | 11.1 | 45.4 | 77.4 | 14.3 | **10.9** |
| Mandelbrot | float64 loops | 15.4 | 16.1 | 16.1 | 16.0 | 15.7 | **14.2** |
| NBody | float64 struct fields via pointers | 8.81 | 10.5 | 16.2 | 1074 | 11.6 | **9.20** |
| FNV32 | uint32 multiply and xor | 10.9 | 11.1 | 13.3 | 26.8 | 25.3 | **11.1** |
| FNV64 | uint64 multiply and xor | 11.2 | 47.9 | 45.6 | 335 | 25.3 | **10.8** |
| BinaryTrees | allocation, GC | 66.7 | 55.1 | **54.4** | 72.8 | 280 | 108 |
| Interfaces | interface method calls | 21.0 | 13.2 | 35.7 | 45.2 | 105 | **30.7** |
| MapInt | map[int]int insert, lookup, delete | 30.5 | 24.5 | 49.4 | **46.1** | 107 | 145 |
| MapString | map[string]int counting | 9.60 | 24.1 | **26.6** | 135 | 32.4 | 80.3 |
| Strings | strings.Builder, strconv, Split, Join | 10.3 | 18.3 | 46.2 | 611 | 35.5 | **13.5** |
| Sort | sort.Ints, sort.Strings | 34.4 | 51.5 | 108 | 192 | 126 | **39.8** |
| JSON | encoding/json Marshal + Unmarshal | 9.63 | 2.92 | 101 | 656 | 29.0 | **28.1** |
| Sprintf | fmt.Sprintf | 16.9 | 10.4 | 160 | 1657 | 70.1 | **46.2** |
| Channels | goroutines, unbuffered channels | 91.0 | — | 113 | 394 | 284 | **31.1** |
| Add (ns/call) | calls from JS: two numbers in, one out | — | 0.59 | **0.59** | 3772 | 6.30 | 2.10 |
| Upper (ns/call) | calls from JS: strings.ToUpper, a string in and out | 149 | 58.2 | 1358 | 15573 | 1201 | **896** |
| Handle (ns/call) | calls from JS: a JSON request handler, a string in and out | 4051 | 1613 | 55864 | 434067 | 17849 | **16108** |
| **Total ms, every kernel once** | | 405* | 329* | 1534 | 11622 | 1475 | **833** |
| **Geometric mean vs native Go** | | 1× | 1.0× | 3.0× | 11.4× | 3.0× | **1.7×** |

Startup (ms from starting to load the output to the first callable function):

| Runtime | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Node.js | 132 | 90.6 | 55.0 | **17.9** |
| Bun | 252 | 297 | 63.2 | **23.1** |
| Chromium | 86.8 | 79.3 | 69.5 | **31.6** |

Output size (all kernels and the standard library they use):

| | goesm | GopherJS | Go wasm | TinyGo wasm |
| --- | ---: | ---: | ---: | ---: |
| Files | `kernels.js` | `bench.js` | `bench.wasm` + `wasm_exec.js` | `bench.wasm` + `wasm_exec.js` |
| Raw | 1194 KiB | 1153 KiB | 4393 KiB | **1104 KiB** |
| gzip -9 | 309 KiB | **228 KiB** | 1211 KiB | 406 KiB |
| brotli -11 | 238 KiB | **169 KiB** | 894 KiB | 299 KiB |
<!-- bench:end -->

What these numbers say about goesm today:

- **Total time: TinyGo is fastest; goesm and Go wasm are close, and GopherJS is far behind.** Running every kernel once takes goesm 1.3–2.8 s, Go wasm 1.3–1.7 s, TinyGo 0.8–1.0 s and GopherJS 9.2–11.6 s. Under Chromium goesm (1.3 s) is ahead of Go wasm (1.7 s), under Node.js they are even (1.5 s), and under Bun goesm is behind (2.8 s, more than a fifth of it FNV64's BigInt arithmetic, which is slow in Bun's engine). In geometric mean against native Go that is 2.6–3.8× for goesm (4.2–4.6× before optimization started), 2.7–3.4× for Go wasm, 1.7–2.0× for TinyGo and 9.1–11.4× for GopherJS.
- **Crossing from JavaScript is free for numbers under goesm, but strings and `encoding/json` still cost more than in wasm.** A goesm function is a JS function, so `Add` costs what it costs in hand-written JS (0.6–0.7 ns); through plain wasm exports TinyGo takes 2–3 ns and Go wasm 5–8 ns (microseconds through `syscall/js`), and GopherJS 3–4 µs. With a string in and out (`Upper`), goesm takes 1.1–1.4 µs per call under Chromium and Node.js (3.0 µs under Bun) and wasm 0.9–1.7 µs (hand-written JS: 0.05–0.07 µs). The JSON request handler takes goesm 48–56 µs per call under Chromium and Node.js (93 µs under Bun) against 16–23 µs in wasm: goesm's `encoding/json`, which works through reflection, is its slowest part.
- **Where goesm stands out and where it lags.** It is the fastest at allocation-heavy BinaryTrees on every runtime, at Interfaces under Bun and Chromium and at the map kernels on some runtimes, and matches hand-written JS on Fib, Mandelbrot, FNV32 and FNV64. It is furthest from native Go on Sprintf and JSON (about 10× under Node.js), standard-library code on byte strings and reflection, and on Sieve, whose `[]bool` is a JS array of booleans. Its output starts in 87–252 ms against 18–70 ms for wasm, and is 238 KiB with brotli against 169 KiB for GopherJS, 299 KiB for TinyGo and 894 KiB for Go wasm.
- **The gap is in goesm's lowering, not in JavaScript.** Hand-written JS is within about 1× of native Go under Node.js and Chromium, so what separates goesm from it is code goesm generates and its runtime, which is where optimization continues.

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
npm ci --prefix test   # tsc and oxlint for TestTSC / TestOxlint (optional locally; required in CI)
go test ./...          # needs Go 1.27+ and Node.js 22.18+; Bun is optional
```

`TestGolden` runs every parameterless exported function of the fixtures under native Go and in the goesm-built ESM and requires equal results; `TestExamples`, `TestJS`, `TestTSC` (strict type-checking of the emitted TypeScript), `TestOxlint` and the opt-in `TestGoConformance` cover the rest. [CONTRIBUTING.md](CONTRIBUTING.md) describes each test and what a pull request needs.

## License

goesm is released under the [BSD 3-Clause License](LICENSE). The output of `emit-ts` and `build` includes the Go standard library packages your code uses, compiled from Go's source, which are under [Go's own BSD-style license](https://go.dev/LICENSE).
