# goesm architecture

[日本語](ARCHITECTURE.ja.md)

goesm is a proof of concept for producing ES Modules from real Go packages: the current Go toolchain is the frontend, goesm lowers Go semantics to TypeScript, and esbuild is the backend.
It is not "a Go-like language compiled to JavaScript". There is no custom syntax, no custom module system and no custom type system.

## 1. Pipeline and responsibilities

```
.go / go.mod / go.sum / go.work
        │  go command + golang.org/x/tools/go/packages   (internal/loader)
        ▼
parse / package load / type check  ── go/parser, go/types (Go is the language authority)
        │
        ▼
Go semantic lowering                ── internal/lower   (the core of goesm)
        │
        ▼
TypeScript (IR) + @goesm/runtime    ── runtime/src/*.ts
        │  esbuild Go API            (internal/build)
        ▼
JavaScript ESM (+ source maps pointing at .go)
```

| layer | owns | does not own |
|---|---|---|
| Go toolchain (`go list` / go/packages / go/types) | module and package resolution, go.mod / go.sum / go.work / GOPROXY, build constraints, parsing, type checking, constant folding, init order | — |
| goesm (`internal/lower`) | mapping Go semantics to TS + runtime calls, type metadata, blocking analysis, the first source map hop (TS→Go) | parsing, type checking, module resolution, JS printing |
| `@goesm/runtime` | Go semantics JS lacks (slices, maps, pointers, interfaces, panic, defer, channels, select, integer wrapping, type descriptors) | typing decisions (go/types settled them at compile time) |
| esbuild (Go API) | TS syntax stripping, JS printing, target lowering, bundling, tree shaking, minification, code splitting, final source maps (composing the TS→Go maps) | any Go-semantic decision |

## 2. Repository layout

```
cmd/goesm/            CLI: goesm build / goesm emit-ts
internal/loader/      go/packages frontend and diagnostics (tagged go list / go/parser / go/types)
internal/lower/       typed AST → TypeScript lowering
  program.go          whole-program analysis (address-taken variables, blocking analysis)
  emit.go decl.go     package = one TS module, type descriptors, struct classes, method tables
  func.go stmt.go     function bodies and statements (defer, switch, select, range, range-over-func ...)
  expr.go types.go    expressions, conversions, operators, type descriptors / zero values
  writer.go           code writer with position markers (source locations kept during codegen)
internal/sourcemap/   TS→Go Source Map v3 builder
internal/build/       pipeline wiring, esbuild Go API, go: specifier resolver
runtime/              @goesm/runtime (TypeScript), embedded into the goesm binary
test/                 end-to-end tests (run in Node.js, golden comparison with native Go)
testdata/             fixture modules (plain Go modules: gofmt / go vet / go test work as usual)
docs/                 GopherJS comparison, example output
```

## 3. Frontend: the Go toolchain as is

* Packages are loaded with `golang.org/x/tools/go/packages` (`NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps`). Module resolution, `go.work`, `GOPROXY` and `go.sum` verification are all the go command's job; goesm reimplements none of it.
* Inputs are Go package patterns (`./main`, `example.com/app/...`). There are no custom imports such as `import "./foo.go"`.
* Target build constraints are `GOOS=js GOARCH=wasm`, the existing port closest to a JS host. `int` is type-checked as 64-bit.
* **No pinned Go version**: go/parser and go/types are linked into the goesm binary, so the newest syntax goesm understands is that of the toolchain goesm was built with. goesm is therefore meant to be built by the toolchain the module selects, via `go tool goesm` (a `tool` directive in go.mod) or `go run`. When the toolchain is newer than goesm, `loader.VersionHint` reports it. This PoC is built with Go 1.27 and its fixtures use Go 1.27 generic methods (`testdata/semantics/generics`).

## 4. Lowering from the AST or from x/tools/go/ssa

**Decision: lower directly from the typed AST (go/ast + go/types).** SSA is not used.

| criterion | typed AST | go/ssa |
|---|---|---|
| 1. keeping up with new Go syntax | anything go/types accepts is usable at once; a new construct needs one lowering case | waits for x/tools' SSA builder (generic methods, range-over-func etc. must be implemented there first) |
| 2. Go semantic accuracy | evaluation order, defer, named results must be handled explicitly | SSA already makes evaluation order explicit (an advantage) |
| 3. implementation size | structured control flow maps onto JS control flow directly | basic blocks + phis need a relooper/stackifier to get back to JS, which is large |
| 4. source locations | AST node positions are embedded as markers directly | positions per instruction are coarse and must be re-associated after restructuring |
| 5. future runtime semantics | async/await, try/finally and labeled break fit JS structure naturally | goroutine resume points push toward state machines |

The weakness in 2 is covered by spilling to temporaries in the lowering where evaluation order matters (multi-assignment, defer argument evaluation, evaluating a range expression once, ...). GopherJS also works from the typed AST.

## 5. Lowering

* **One Go package = one TypeScript module = one ES module.** A Go import becomes `import * as mathx from "go:example.com/app/mathx"`. The `go:` scheme only separates Go packages from npm packages; the import path itself is kept.
  * `goesm build` (default) has esbuild produce one bundle (`dist/main.js`).
  * `goesm build -split` emits one module per package, e.g. `dist/example.com/app/mathx.js`, and Go imports remain ESM dependencies such as `import * as mathx from "./mathx.js"` (the runtime is `dist/@goesm/runtime.js`).
* Names: Go identifiers never contain `$`, so every name goesm introduces does (`User$type`, `User$Adult`, `$rt`, `$t3`). Each Go object within a function declaration gets a unique JS name, so Go shadowing never has to be reproduced with JS scoping rules.
* Constant expressions are emitted as the values go/types computed (iota, typed constants, `unsafe.Sizeof`, ...).
* Package variables are initialised in `types.Info.InitOrder` order, then `init()` runs, then `main()` if the entry package has `func main`.
* TypeScript annotations in the generated code are for readability and debugging only; nothing type-checks them (esbuild does not).

### Value representation

| Go | JS representation | notes |
|---|---|---|
| bool, float64 | boolean, number | |
| float32 | number (rounded with `Math.fround`) | |
| int8/16/32, uint8/16/32 | number, wrapped after every operation (`\|0`, `>>>0`, `<<24>>24`, `Math.imul`) | exact |
| int, int64, uint, uint64, uintptr | number | **inexact above 2^53, no 64-bit wrap-around** (known difference) |
| string | JS string with one code unit per byte | `len`, indexing, slicing, comparison and invalid UTF-8 match Go; converted at the JS boundary with `toJSString` / `fromJSString` |
| struct | instance of a generated class (`$clone` / `$set`) | value copies are inserted by the lowering; the object identity is the address |
| array | JS array | copied explicitly, like structs |
| slice | `Slice{$array,$offset,$length,$capacity}`, nil is `null` | append / re-slice aliasing as in Go |
| map | `GoMap` (JS `Map` + hash keys with Go equality), nil is `null` | struct / interface / NaN keys, nil-map panics |
| pointer | `*struct` / `*array` is the object itself; otherwise an object with a `.v` accessor (`Cell` / `FieldPtr` / `IndexPtr`) | `&x == &x` and `&s.f == &s.f` guaranteed by caching |
| interface | `Iface{t: type descriptor, v: value}`, nil is `null` | keeps the dynamic type: `MyInt(1)` ≠ `int(1)`, an interface holding a nil `*T` is non-nil |
| func | JS function | |
| chan | runtime `Chan` | |
| type parameter | the representation of its type argument (erasure) | type descriptors arrive as dictionary parameters |

### Type metadata

Every named type has a runtime descriptor (`$rt.named(pkgPath, name)`) with its underlying type, fields (name, pkgPath, tag, embedded) and value / pointer method sets (method names and signature descriptors). Composite types (`[]T`, `map[K]V`, `func(...)`, `struct{...}`, `interface{...}`) are memoized by structure, so **descriptor identity is Go type identity**. Interface checks use these method tables and never rely on TypeScript structural typing. Unexported methods are qualified with their pkgPath.

### Generics: type erasure + runtime type dictionaries

* One copy of each function or method is generated (erasure); type arguments are passed as leading **dictionary parameters** holding runtime type descriptors: `First($T_T, values)`.
* With descriptors available, zero values (`var x T`), interface conversion (`any(x)`), `==`, aggregate copies, `new(T)` and `make([]T)` all behave correctly for each type argument.
* Instances of generic types (`Stack[Pair[string,int]]`) are memoized descriptors too, so `a.(Pair[string,int])` and `a.(Pair[string,string])` are told apart.
* Go 1.27 generic methods receive the receiver's type arguments, then the method's own. They cannot satisfy interfaces, so they are not in method tables.
* Why: specialization multiplies code per type argument, which is bad for JS bundles. Keeping only TS generics leaves no type information at run time, so zero values, interface conversion and reflect would be impossible. Erasure + dictionaries is the same idea as gc's GC-shape stenciling with dictionaries; it is the simplest option and leads to reflect. `<T>` is kept in the TS for readability only.

### defer / panic / recover

```ts
function F() {
  let $r0 = zero;                 // result variables (named results keep their names)
  const $d = new $rt.Defers();
  $body: try {
    ...; $r0 = expr; break $body; // return assigns results, then defers run
  } catch ($e) { $d.fail($e); } finally { $d.run(); }
  return $r0;                     // returns values the defers may have changed
}
```

* A panic throws a `GoPanic` (a JS Error). The panic value is kept as an interface value; runtime errors have types implementing `runtime.Error` (`Error()` / `RuntimeError()`). A JS `TypeError` (touching null) becomes the nil pointer dereference runtime error.
* The function value and arguments of a deferred call are evaluated at the defer statement and captured in a closure.
* recover looks at the frame whose deferred call is currently running synchronously.
* From JS, an unrecovered panic is a `GoPanic` exception; with `--enable-source-maps` its stack points at `.go` lines (tested).

### goroutines / channels / select

* **Blocking analysis** (whole program): a function is blocking if it contains channel operations, a select without default, a range over a channel, a call or defer of a blocking function, or a dynamic call that may reach one. Blocking functions become `async function`s and every blocking point is an `await`; all other functions stay synchronous (no await cost). Dynamic calls are resolved conservatively by signature (function values) or method name (interfaces).
* `go f(x)` evaluates the function value and arguments in place; `$rt.go(closure)` starts it as a microtask.
* Channels are a buffer plus send/receive wait queues in the runtime. An operation that can complete immediately returns synchronously; only a blocking one returns a Promise. Implemented: unbuffered handoff, close (including panicking blocked senders), and `select` (uniformly random among ready cases, default, nil channels block forever).
* So "converting to async/await makes it Go" is not the assumption. Blocking semantics live at the runtime boundary (the wait queues); async/await is only the mechanism to suspend and resume a goroutine. Deadlock detection, Goexit, goroutine-local panic state, timers and `sync` will be built on that boundary.
* JS boundary: a blocking exported function returns a Promise (`await Example()` is 42).

## 6. Runtime (`runtime/src`)

| file | responsibility |
|---|---|
| `types.ts` | type descriptors (reflect.Kind numbering, memoized named / composite types, method tables, generic instances, `error`) |
| `iface.ts` | interface values, box / assert / type switch, `==`, map hash keys |
| `slice.ts` | slices, append / copy / bounds checks, indexing type parameters without a core type |
| `map.ts` | Go maps |
| `ptr.ts` | Cell / field pointers / element pointers, load / store through type parameters |
| `string.ts` | byte strings ⇔ UTF-8 / runes, JS boundary conversion |
| `int.ts` | integer division and remainder (divide-by-zero panic), shifts, 64-bit bitwise ops, min / max |
| `panic.ts` | GoPanic, runtime error types, Defers, recover |
| `chan.ts` | channels, select, goroutine start |
| `interop.ts` | Go value → JSON-shaped JS value guided by descriptors (golden tests, future JS ABI) |

Only what the fixtures need is implemented; no scheduler or reflect was built ahead of time.

## 7. reflect / unsafe / memory representation

* **reflect**: codegen always keeps named type identity, field names, tags and embedding, method sets (names and signatures) and type arguments in runtime descriptors. `reflect.TypeOf` can be the `t` of an interface value, `reflect.Value` a pair of (descriptor, value or pointer object), and `Kind` already uses reflect's numbering. The reflect package itself is planned as a target replacement of its Go source (see below), swapping its `internal/abi` dependencies for descriptors.
* **Pointer representation**: currently "aggregates are the object itself, everything else is an accessor object". All pointer creation goes through `ptr.ts`, so the representation can be replaced when `unsafe.Pointer` interop arrives.
* **unsafe / linear memory**: the architecture does not close with "JS has no pointers, so unsupported". The planned direction, step by step: (1) back numeric slices such as `[]byte` with TypedArrays (the slice backing store is private to `slice.ts`); (2) represent `unsafe.Pointer` as a tagged (ArrayBuffer, byte offset) or (object, field) value and implement `unsafe.Slice` / `unsafe.String` / `unsafe.Add` on DataView; (3) lay out structs in linear memory (ArrayBuffer) only for packages that need it. Today, uses of `unsafe` produce a "not supported yet" goesm diagnostic.

## 8. Source maps and diagnostics

* The lowering embeds Go position markers in expression and statement strings; the writer records a mapping once the output column is known. Location information is never dropped during codegen.
* Each generated TS module carries an inline TS→Go Source Map v3. esbuild reads it and composes the final **JS→.go** map (with Go source in `sourcesContent`). A test checks that with Node's `--enable-source-maps` a panic's stack points at `panics.go:NN`.
* Diagnostics are tagged by layer:
  * `file.go:4:17: ... [go/types]` / `[go/parser]` / `[go list]` — errors from the Go frontend, at the original `.go` position.
  * `file.go:8:3: goto is not supported yet [goesm lowering]` — a goesm limitation, shown separately from Go compile errors.
  * `internal error: esbuild rejected TypeScript generated by goesm ... [esbuild]` — a goesm bug, never disguised as a Go error.

## 9. Standard library

The policy is to compile the ordinary Go source, not to start by porting packages to TS by hand.

Current status (reproduce with `go test ./test -run TestStdlibStatus -v`):

* `unicode`, `unicode/utf8` and `math/bits` compile from Go source and match native Go in the golden tests (`testdata/semantics/stdlibuse`).
* `strings`, `strconv`, `slices`, `maps` and `encoding/json` have few problems in their own code (goto, `unsafe.String`, `unsafe.Pointer`, the `maps.clone` linkname, ...), but through `errors` → `internal/reflectlite`, `iter` → `runtime` and so on they depend on **`runtime`, `internal/abi`, `internal/reflectlite`, `internal/runtime/*` and `sync/atomic`**, which produce thousands of diagnostics.

Those need target-specific replacements, like GopherJS's natives. goesm plans to keep the replacements as **Go source** too and pass them to go/packages through `packages.Config.Overlay`: the package graph stays visible to Go tooling, and the replacements are a fixed set of files owned by goesm that dependencies can never extend.

## 10. Tooling compatibility and security

* `.go` files are plain Go: no goesm-specific syntax, directives or magic comments. The fixtures pass `go vet` / `go build` / `go run`, and the golden tests compare against exactly that native execution. The package graph is the one the go command resolved, so call graphs for govulncheck and similar tools are unchanged.
* Importing a dependency never runs code inside goesm. There is no compiler plugin or extension mechanism; the only esbuild plugin is goesm's built-in resolver (`go:` and `@goesm/runtime`).
* Concerns: (1) go/packages runs `go list`, so goesm inherits the go command's trust boundary for its environment (`GOFLAGS` etc.), `go.work` and fetching modules from `GOPROXY` (goesm does not widen it). (2) Generated code relies on Go's type safety; a lowering bug shows up as wrong behaviour, not memory unsafety (JS itself is memory safe). (3) Generated ESM uses host APIs such as `globalThis.reportError`; DOM API bindings are not implemented. (4) `GoPanic` messages and the source map's `sourcesContent` include Go source, so a published bundle ships the source (an option to drop `SourcesContent` is not implemented).

## 11. Implemented / not implemented / differences from native Go

**Implemented (verified by golden tests against native Go)**: package import, functions, multiple results, named results, closures, structs (value copies, methods, pointer methods, embedding and promotion, comparison), arrays, slices (aliasing, append, copy, re-slicing, nil), maps (struct / interface keys, comma-ok, delete, nil maps, range), pointers (variables, fields, elements, `new`, identity), defer (ordering, modifying named results, LIFO), panic / recover (runtime errors, re-panic), interfaces (dispatch, type assertions, type switches, comparison, nil interface vs nil pointer), generics (generic functions, constraints, generic types, Go 1.27 generic methods, type identity), method values / method expressions, switch / fallthrough / labeled break and continue, range over int, range-over-func (break / continue / return), Go 1.22 per-iteration loop variables, 8/16/32-bit integer wrap-around, integer divide-by-zero panics, UTF-8 strings and runes, goroutines, unbuffered / buffered channels, close, range over channels, select (including default), package variable init order and `init()`.

**Not implemented** (produces a goesm diagnostic or does not work):
* exact 64-bit integers (BigInt or hi/lo), complex64/128
* `unsafe`, `reflect`, the `runtime` replacement, `sync` / `sync/atomic`, `time`, and most of the stdlib including `fmt` / `strconv` / `strings`
* `goto`; blocking, defer or labeled branches inside a range-over-func body; taking the address of type-parameter-typed variables; local types depending on type parameters; conversion from a slice to an array pointer (`(*[N]T)(s)`)
* deadlock detection ("all goroutines are asleep"), goroutine preemption, `runtime.Goexit`, goroutine-local recover state
* a JS calling ABI (automatic Go ⇔ JS value conversion), DOM / `syscall/js` bindings
* shared loop variables in range loops for files with `go` < 1.22

**Known differences from native Go** (`TestKnownGaps` asserts that the first two still differ, via `Uint64Wrap`, `Int64Precision` and `AppendCap`; the rest are not deterministic enough to pin and are documented only):
* `int`/`int64`/`uint64` are inexact above 2^53 and do not wrap on 64-bit overflow (`uint64(0)-1` is `-1`).
* `append` capacity growth is approximated (no size-class rounding), so `cap()` can differ from gc.
* Map range order is insertion order (Go randomises it; both are unspecified).
* `recover()` also works when called indirectly from a deferred function (Go requires a direct call); after an await it returns nil.
* Goroutines switch only at blocking points (cooperative). Blocking exported functions return Promises to JS.
* Blocking of dynamic calls is decided conservatively by signature / method name, which can add unneeded `await`s (behaviour is unchanged).
* `print` / `println` write to stderr in the Go runtime's format, but pointer, map, channel, func, slice and interface values print a fixed address instead of a real one.

## 12. Next three items

1. **Target replacements for `runtime` and friends as Go source overlays**, so `errors` / `strconv` / `strings` / `slices` / `maps` / `sync` compile from their Go source (including minimal `unsafe.String` / `unsafe.Pointer` support). Most of the stdlib is blocked here, so this comes first.
2. **Exact 64-bit integers** (`int64`/`uint64` as BigInt or hi/lo, and a decision for `int`) and complex numbers. This widens what the golden tests can cover at once.
3. **Completing the goroutine runtime**: deadlock detection, goroutine-local panic / recover state (recover across async boundaries), `sync.Mutex` / `WaitGroup` / `time` timers, and a JS calling ABI (converting arguments and results of exported functions).
