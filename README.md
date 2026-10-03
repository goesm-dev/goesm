# goesm

[日本語](README.ja.md)

Compile Go packages into native ES modules for the modern web.

`goesm` lets you write ordinary Go packages, use the Go module ecosystem, and compile them into JavaScript modules that can be consumed by Vite, Vue, Astro, or any other ESM-based toolchain.

```go
package cart

type Item struct {
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
```

The goal is not to introduce a Go-like language for the browser.

The input is Go.

`go.mod`, `go.sum`, package imports, generics, interfaces, pointers, maps, goroutines, channels, `defer`, `panic` / `recover`, reflection, and other Go semantics belong to the Go side of the system.

## Architecture

`goesm` uses the Go toolchain as the source-language authority.

```text
.go
 │
 ▼
Go parser / type checker / package loader
 │
 ▼
goesm
Go semantics → TypeScript
 │
 ▼
TypeScript
 │
 ▼
Vite / Rolldown / another host toolchain
 │
 ▼
ES Modules
```

TypeScript is goesm's output and the host toolchain's input: generated ES modules, one per Go package, that you do not write or edit by hand.

`goesm` does not attempt to replace the Go parser, Go type system, Go Modules, or the JavaScript build ecosystem.

Instead, it focuses on lowering Go semantics into a representation that modern web tooling already understands.

## Go packages, not Go programs

A `goesm` project does not need a `main` package.

This is intentional.

A Go package can itself become an ES module:

```text
example.com/app/cart
        │
        ▼
      goesm
        │
        ▼
   JavaScript ESM
```

The application entry point may live elsewhere.

For example:

```text
index.ts
   │
   ├── Go package
   ├── Go package
   └── Go package
```

or:

```text
Astro / Vue / Vite
        │
        ▼
   Go packages
```

This makes the model closer to the TypeScript ecosystem: packages provide reusable modules, while the surrounding application decides where execution begins.

`package main` may still be useful for standalone Go applications, but it is not the fundamental compilation unit of `goesm`.

The fundamental unit is the **Go package**.

## Example structure

```text
app/
├── go.mod
├── src/
│   ├── cart/
│   │   ├── cart.go
│   │   ├── price.go
│   │   └── model.go
│   └── user/
│       └── user.go
└── web/
    └── index.ts
```

Go packages remain ordinary Go packages:

```go
import "example.com/app/src/cart"
```

No `.go` file imports, custom module syntax, or JavaScript-style relative imports are introduced.

## Goals

`goesm` aims to preserve the existing Go development experience:

```text
go.mod
go.sum
go.work
go fmt
gopls
go test
go vet
golangci-lint
govulncheck
```

The compiler should reuse the Go ecosystem instead of creating parallel tooling.

At the JavaScript boundary, it should reuse modern web tooling instead of implementing another bundler.

## Non-goals

`goesm` is not:

- a Go-inspired language
- a WebAssembly runtime
- a new package manager
- a replacement for Vite or Rolldown
- a framework such as Vue or Astro

Framework-specific integration belongs in separate adapters.

For example, `gosfc` can connect:

```vue
<script setup lang="go">
import "example.com/app/src/cart"

total := cart.Total(items)
</script>
```

to `goesm`, while Vue remains responsible for the SFC and template layer.

## Status

`goesm` is currently experimental.

The initial work focuses on proving that ordinary, type-checked Go packages can be lowered to TypeScript and consumed as native ES modules while preserving Go semantics and package boundaries.

The proof of concept in this repository loads packages with go/packages + go/types and lowers them to a tree of ESM-ready TypeScript files, one per Go package, plus a small runtime written in TypeScript (`@goesm/runtime`). Any bundler (Vite, Rolldown, esbuild) or TypeScript-aware runtime (Bun, Node.js with type stripping) consumes that tree directly. `goesm build` bundles the same tree with esbuild's Go API as a convenience. See [ARCHITECTURE.md](ARCHITECTURE.md) for the design, what is implemented and what is not, [docs/gopherjs-comparison.md](docs/gopherjs-comparison.md) for how it differs from GopherJS, and [docs/example-output.md](docs/example-output.md) for generated TypeScript and JavaScript.

### Install

goesm needs the Go toolchain (it loads packages with `go list`), Go 1.27 or later; an older `go` downloads 1.27 by itself through `GOTOOLCHAIN`. It does not need Node.js or npm: the runtime (`@goesm/runtime`) is embedded in the binary and written into the output (as `<dir>/@goesm/runtime/` by `emit-ts`, bundled by `build`), so there is no npm package to install.

The recommended way is to add goesm as a tool of your module, so that it is built with the same toolchain as your code:

```sh
go get -tool github.com/goesm-dev/goesm/cmd/goesm@latest
go tool goesm emit-ts ./cart     # goesm-ts/<import path of cart>.ts + goesm-ts/@goesm/runtime/
go tool goesm build ./cart       # or a bundled dist/cart.js
```

Or install it on your `PATH`:

```sh
go install github.com/goesm-dev/goesm/cmd/goesm@latest
goesm emit-ts ./cart
```

There are no prebuilt binaries: goesm runs `go` anyway, and building it with your own toolchain keeps its go/types in step with the Go your module uses. `goesm version` prints the goesm version and the Go it was built with. Release notes are on [GitHub Releases](https://github.com/goesm-dev/goesm/releases).

While goesm is experimental, releases are prereleases named `v0.0.1-beta.N`; `@latest` resolves to the newest one, and `@v0.0.1-beta.1` pins one.

### Usage

Inputs are ordinary Go package patterns in an ordinary Go module. `goesm emit-ts` writes the TypeScript tree:

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
    ├── natives.ts
    └── ...                    the other runtime files
```

The module of Go package `p` is `<dir>/<p>.ts` (standard library packages too: `strings.ts`, `internal/bytealg.ts`), and the runtime is `<dir>/@goesm/runtime/`. A Go import path cannot start with `@`, so the runtime never collides with a package. Modules import each other with relative specifiers ending in `.ts`, so no resolver, plugin or bundler configuration is needed. Import the entry package from your own code:

```ts
// index.ts in a Vite project, or run directly: bun index.ts / node index.ts (Node.js 22.18+)
import { Result } from "./goesm-ts/example.com/app/main.ts";
console.log(Result()); // 3
```

To type-check code importing the tree with `tsc`, enable `allowImportingTsExtensions`.

`goesm build` is a convenience (and what the tests use): it writes the same tree to a temporary directory and bundles it with esbuild's Go API.

```sh
go run ../../cmd/goesm build ./main          # dist/main.js (+ .js.map pointing at .go)
go run ../../cmd/goesm build -split ./main   # one ES module per Go package: dist/example.com/app/main.js, ...
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

[examples/](examples) has runnable examples (the cart above, standard library use, goroutines) with the JS that imports them.

Standard library packages are compiled from their own Go source; the few tied to the gc runtime (`runtime`, `internal/reflectlite`, `sync`) are replaced by goesm's own Go source. A standard library function goesm cannot lower yet becomes a stub that panics if called, and `build` / `emit-ts` print how many there are (`-v` lists them).

goesm uses go/types from the toolchain it was built with, so build it with the toolchain your module uses (for example via a `tool` directive and `go tool goesm`). This repository's fixtures use Go 1.27.

### Tests

```sh
mise install           # Go, Node.js and Bun at the versions pinned in mise.toml (CI uses the same)
npm ci --prefix test   # tsc and oxlint for TestTSC / TestOxlint (optional locally; required in CI)
go test ./...          # needs Go 1.27+ and Node.js 22+; Bun is optional
```

- `TestGolden` runs every parameterless exported function of the fixtures under native Go and in the goesm-built ESM (Node) and requires equal results.
- `TestJS` runs `test/js/*.test.mjs` (node:test) against built bundles.
- `TestKnownGaps` pins the documented differences from native Go.
- `TestExamples` builds `examples/*`, runs each `index.mjs` under Node (and Bun when installed) and compares with its `output.txt`.
- `TestOxlint` lints the ESM built from the fixtures (bundle and split) with oxlint's correctness rules and fails on any finding; `test/lint_test.go` lists the four rules turned off for generated code and why.
- `TestStdlibStatus -v` reports which standard library packages lower and how many of their functions are stubs.
- `TestTSC` type-checks the emitted TypeScript (fixtures, examples, runtime) with tsc in strict mode, together with a consumer that checks exported Go APIs have their Go types in TypeScript. Like `TestOxlint` it needs `npm ci --prefix test` and is required in CI. Comparing results with native Go stays the semantic gate.
