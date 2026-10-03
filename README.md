# goesm

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

TypeScript is an implementation detail and an intermediate target.

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
