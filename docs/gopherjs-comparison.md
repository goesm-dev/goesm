# Comparison with GopherJS

[日本語](gopherjs-comparison.ja.md)

GopherJS is the most mature Go→JavaScript implementation and the main reference. goesm is not a GopherJS clone. By targeting TypeScript as an intermediate form and using esbuild as the backend, it hands off what GopherJS implements itself and simplifies the rest with modern JS primitives.

The GopherJS column summarises its upstream README and compatibility documents and the design of its compiler and prelude (master as of 2026-10; each GopherJS release targets one Go release, currently the Go 1.21 line).

| aspect | GopherJS | goesm (PoC) | simplified / changed in goesm |
|---|---|---|---|
| package loading | its own build package (based on go/build, with module support); the stdlib is loaded together with the natives overlay | fully delegated to `golang.org/x/tools/go/packages` (the go command resolves modules / go.work / GOPROXY); the replaced stdlib packages are substituted while go/packages parses them (`ParseFile`) | no module or build resolution code |
| Go version | each release supports one Go version (natives depend on the stdlib version) | goesm does not pin a version: go/types from the toolchain that built goesm is the frontend (`go tool goesm` follows the module's toolchain). Go 1.27 generic methods pass | new syntax is accepted as soon as go/types accepts it; only lowering cases need adding. The Go-source replacements (`runtime`, `internal/reflectlite`, `sync`) only follow the API the rest of the stdlib uses, not its internals |
| AST / type info | generates JS directly from go/ast + go/types (own analyses: blocking, escape, ...) | also lowers the typed AST directly (no SSA; see ARCHITECTURE.md §4) | the output is a TS IR, not JS: no JS printer, minifier or bundler |
| output format | one script (its own `$packages` registry), its own dead code elimination | one Go package = one ES module; `go:` specifiers are bundled by esbuild, or one ESM per package with `-split` | ESM, tree shaking, code splitting, minification and target lowering are esbuild's |
| integers | `int` is 32-bit (emulates a 32-bit platform); `int64`/`uint64` are exact as high/low pairs | `int` is 64-bit as in Go's wasm type checking; 8–32-bit are exact; 64-bit are JS numbers (inexact above 2^53) | **GopherJS is more exact**; goesm's next item (BigInt or hi/lo) |
| strings | JS strings treated as byte sequences | the same (one code unit = one byte) | — |
| maps | runtime `$keyFor` turns keys into strings and stores `{k, v}` in a JS object | JS `Map` + hash keys from type descriptors (primitives are used as keys directly; structs / interfaces / NaN are serialised for Go equality) | primitive keys need no stringification |
| pointers | non-struct pointers are objects with getter / setter closures; a struct pointer is the struct object | nearly the same idea: `.v` accessor Cell / FieldPtr / IndexPtr, structs / arrays are the object itself, identity guaranteed by a WeakMap cache | creation goes through a few runtime functions (replaceable for future unsafe) |
| interfaces | non-struct values are wrapped in `T.wrapped`; structs get their dynamic type from the constructor; methods live on JS prototypes | always `Iface{t, v}` (dynamic type descriptor + value); dispatch through the descriptor's method table | one uniform representation, no reliance on prototype chains |
| goroutines | functions marked by blocking analysis become resumable state machines (a `$s` switch and saved frames) resumed by its own scheduler | functions marked by a similar blocking analysis become **`async function`s** with `await` at blocking points; goroutines start as microtasks | no state machine generation or stack saving; the JS engine's async stack traces work as is; neither preempts |
| channels | runtime send / recv queues and `$select` | runtime wait queues; immediate completion is synchronous, a Promise only when blocking | blocking semantics stay at the runtime boundary while Promises express suspension |
| defer / panic / recover | per-goroutine defer stack and panic state in the runtime, tied into the state machine | a `Defers` frame per function with JS `try/catch/finally` + a labelled `break` | named result updates and defers after return are expressed with JS control flow; goroutine-local recover state is not implemented |
| reflect | reflect implemented in natives; detailed metadata on type objects | type descriptors (kind, fields, tags, method sets, type arguments) are always emitted; `internal/reflectlite` is replaced by Go source over them (enough for `errors`, `sort`); `reflect` itself is not implemented | reflect is planned the same way, as Go source over the descriptors |
| runtime | a large hand-written JS prelude + natives replacing stdlib code | a small TS runtime (`@goesm/runtime`, only what the fixtures need); `runtime`, `internal/reflectlite` and `sync` are replaced by Go source; functions without a Go body are natives, one ES export each | the runtime and the natives are tree-shaken by esbuild too |
| source maps | its own JS printer emits JS→Go maps directly | goesm only builds TS→Go maps; esbuild composes them into JS→Go | final map generation, composition and minification tracking are esbuild's |
| unsafe | only a small part | the patterns the stdlib needs: `unsafe.Pointer` round trips, `unsafe.String` / `unsafe.Slice` over slice elements; reinterpreting memory is a diagnostic (an ArrayBuffer / DataView representation is planned, ARCHITECTURE.md §7) | — |
| generics | supported (type arguments passed to the runtime) | erasure + type descriptor dictionary parameters | — |

## Summary

* Ideas taken from GopherJS: lowering directly from the typed AST, limiting goroutine support to the functions that need it through blocking analysis, strings as byte sequences, struct pointer = struct object.
* Simplified by TypeScript + esbuild: the JS printer, bundler / output format, minification, tree shaking, final source map generation, ES target support.
* Simplified by modern JS primitives: goroutine state machines → async/await, defer → try/finally, maps → JS `Map`, package registry → ES modules.
* Where GopherJS is ahead: 64-bit integers, reflect, stdlib coverage, `syscall/js`, deadlock detection.
