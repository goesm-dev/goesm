# Changelog

[日本語](CHANGELOG.ja.md)

The changes in each goesm version, newest first. Every tagged version is a semver prerelease, so `go install github.com/goesm-dev/goesm/cmd/goesm@latest` picks the newest one. The notes of each GitHub release are its section here and in [CHANGELOG.ja.md](CHANGELOG.ja.md); [docs/releasing.md](docs/releasing.md) explains how.

## Unreleased

Changes on `main` since v0.0.1-beta.3: [v0.0.1-beta.3...main](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.3...main).

These changes make the generated code faster and its bundles smaller, so that it comes closer to hand-written JavaScript. Nothing changes how Go code or its JavaScript callers are written.

### Performance

- An interface value of a type whose methods are called through interfaces is an instance of a box class that holds the type's methods. An interface call is a method call on that value, and a pointer to a struct is its own interface value, so converting it allocates nothing. [#86](https://github.com/goesm-dev/goesm/pull/86) [#88](https://github.com/goesm-dev/goesm/pull/88)
- A run of assignments to one int64 or uint64 field works on a local copy that is stored back once, so V8 keeps the value in a machine word. Rand64 goes from 33 to 14 ms on Node.js, as fast as hand-written JavaScript with BigInt. [#78](https://github.com/goesm-dev/goesm/pull/78)
- `iter.Pull` and `Pull2` step simple sequence literals as JavaScript generators instead of switching goroutines. fmt, encoding/json and sort stay synchronous in programs that use `iter.Pull`. [#80](https://github.com/goesm-dev/goesm/pull/80) [#83](https://github.com/goesm-dev/goesm/pull/83)
- The array, offset and length of a slice that a loop indexes are loaded once before the loop. NBody goes from 1.20× to 1.03× hand-written JavaScript on Node.js. [#87](https://github.com/goesm-dev/goesm/pull/87)
- `json.Unmarshal` into a pointer of a known type decodes without boxing, and JSON arrays are made at their exact length. Allocation per call against hand-written JavaScript goes from 2.33× to 1.84× for the JSON kernel and from 2.16× to 1.66× for Handle. [#89](https://github.com/goesm-dev/goesm/pull/89)
- `slices.Sort` sorts integers that fit in 32 bits in an Int32Array and string slices in place. The Sort kernel goes from 1.12× to 1.00× hand-written JavaScript on Node.js and from 1.23× to 1.07× on Bun. [#90](https://github.com/goesm-dev/goesm/pull/90)
- Benchmark at [#84](https://github.com/goesm-dev/goesm/pull/84): goesm's geometric mean is 1.03–1.10× native Go, and its total time is the shortest of the four compilers on every runtime. [#84](https://github.com/goesm-dev/goesm/pull/84)

### Bundle size

- A program whose regexp patterns are all known at compile time matches them with the engine's RegExp when the translation finds the same matches in linear time, and regexp's parser and engines are left out. `strconv.Atoi`, `strings.TrimSpace` and `strings.Split` no longer pull in unicode's tables or NumError's methods. A calendar package that parses dates grows by 1.9 KB gzip with strings and strconv, down from 10.2 KB, and by 4.5 KB with a regexp, down from 55.6 KB. [#85](https://github.com/goesm-dev/goesm/pull/85)
- Method tables keep a method only when reachable code calls it through an interface, as Go's linker does. unicode's category and script tables are dropped when no regexp pattern can use them. A Markdown bundle goes from 215 to 163 KiB gzip. [#82](https://github.com/goesm-dev/goesm/pull/82)
- More package variable initializers count as side-effect free, empty init functions are not emitted, and strconv computes its 128-bit powers of ten on demand. The neverthrow comparison's standard library bundle goes from 101 to 52 KiB gzip. [#81](https://github.com/goesm-dev/goesm/pull/81)
- `fmt.Errorf` and `fmt.Sprintf` with constant formats turn `%w`, `%q` and error operands into string concatenation, and struct descriptors are more compact. The Vue comparison's size goes from 2.16× to 1.22× Vue's. [#79](https://github.com/goesm-dev/goesm/pull/79)

### Fixes

- Exported struct fields whose names start with a non-ASCII upper-case letter are exported, and unexported ones with non-ASCII names are not. [#81](https://github.com/goesm-dev/goesm/pull/81)

### Comparison suite and CI

- compare/ follows Vue 3.5's reactivity algorithm and adds Go packages written with bundle size in mind next to the standard library versions. [#79](https://github.com/goesm-dev/goesm/pull/79) [#81](https://github.com/goesm-dev/goesm/pull/81)
- CI runs of a pull request no longer cancel the push run on `main` after a merge. [#77](https://github.com/goesm-dev/goesm/pull/77)

## v0.0.1-beta.3

Tagged on 2026-10-06. Changes since v0.0.1-beta.2: [v0.0.1-beta.2...v0.0.1-beta.3](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.2...v0.0.1-beta.3).

JavaScript calls Go exports with plain JavaScript values, bundles are much smaller, and syscall/js calls are fast enough for DOM code. The new comparison suite measures goesm against popular JavaScript libraries, and the speedups it led to are part of this version.

### Breaking changes

- **Exported functions and methods take and return plain JavaScript values.** [#68](https://github.com/goesm-dev/goesm/pull/68)
  - A string is a JS string, a slice is an array, a struct is a plain object whose properties are named as encoding/json names the fields, and a map with string keys is a plain object, any other map a Map.
  - A final `error` result is thrown as a `GoError`. Passed back to Go, the `GoError` is the Go error again.
  - A function returning an `http.Handler` gives a fetch handler: `export default { fetch: Handler() }`.
  - Pointers to struct types with methods, non-empty interfaces and channels pass as handles whose methods convert in the same way.
  - TypeScript sees the JavaScript types.
  - **Migration:** remove the hand-written conversions. Calls of `rt.fromJSString`, `rt.toJSString`, `rt.sliceLit`, `rt.toArray` and `rt.icall(err, "Error")` must go, and the entry module no longer re-exports the runtime as `$runtime`. See [docs/js-exports.md](docs/js-exports.md).

### New features

- `syscall/js` calls whose arguments are booleans, numbers, strings, `js.Value`, `js.Func` or `nil` compile to direct JavaScript property accesses and calls. `add(1, 2)` through syscall/js goes from 173 to 23 ns on Node.js. Libraries written for `GOOS=js`, such as `honnef.co/go/js/dom/v2`, work unchanged. [docs/dom.md](docs/dom.md) explains how to use the DOM from Go. [#72](https://github.com/goesm-dev/goesm/pull/72)
- compare/ does the same work in a Go package built by goesm and in JavaScript with a popular library, and reports gzip size and time under Node.js and Bun. It covers luxon, neverthrow, connect-es, React, VitePress's markdown-it, Astro's remark, Vue's reactivity and Tailwind CSS. [#71](https://github.com/goesm-dev/goesm/pull/71) [#76](https://github.com/goesm-dev/goesm/pull/76)

### Bundle size

- Method tables list only the methods that can be called dynamically, as Go's linker does. Package variables whose initializers have no side effects are dropped when unused, and time reads the host's UTC offset without syscall/js. [#70](https://github.com/goesm-dev/goesm/pull/70)
- The entry module no longer re-exports the runtime. [#68](https://github.com/goesm-dev/goesm/pull/68)

| `goesm build -minify`, gzip -9 | v0.0.1-beta.2 | v0.0.1-beta.3 |
| --- | ---: | ---: |
| No standard library | 23,619 B | 3,561 B |
| Six `time` functions | 47,774 B | 13,510 B |
| `fmt.Sprintf` | 140,505 B | 94,778 B |
| `regexp`, `strconv` and `time` | 131,053 B | 93,887 B |

### Performance

- fmt's `Sprintf` and `Errorf` format errors and Stringers with `%v` and `%s`, and `Errorf`'s one `%w`, by string concatenation. `range []byte(s)` ranges over the string without copying it, and `utf8.Valid` no longer reads 64-bit words. [#71](https://github.com/goesm-dev/goesm/pull/71)
- Non-ASCII strings of 64 bytes and more cross the JavaScript boundary through the engine's UTF-8 encoder and decoders. More bounds checks are left out, and range loops over slices of structs read the fields they use instead of copying each element. [#73](https://github.com/goesm-dev/goesm/pull/73)
- Zero values are module constants, struct pointers converted to interfaces reuse their interface value, and field and element pointers are cached on their object instead of in WeakMaps. [#74](https://github.com/goesm-dev/goesm/pull/74)
- A function that blocks only through some of its arguments gets a synchronous version for calls whose arguments cannot block. `fmt.Sprintf` no longer becomes async in programs that import net/http. [#75](https://github.com/goesm-dev/goesm/pull/75)
- In the comparison suite, [#71](https://github.com/goesm-dev/goesm/pull/71) takes the Vue workload from 102 to 42 ms, [#73](https://github.com/goesm-dev/goesm/pull/73) brings VitePress (goldmark) to 0.62× its earlier time on Node.js and 0.47× on Bun, [#74](https://github.com/goesm-dev/goesm/pull/74) makes the Vue workload a further 2.2–3.2× faster, and [#74](https://github.com/goesm-dev/goesm/pull/74) and [#75](https://github.com/goesm-dev/goesm/pull/75) each make connect-es 10–20% faster.

### Fixes

- protobuf's repeated message fields lost elements: a response with 10 messages came back with some of them nil. [#69](https://github.com/goesm-dev/goesm/pull/69)
- When the fast path of `fmt.Sprintf` fell back to fmt's own code, it called the operands' `Error` and `String` methods twice. [#73](https://github.com/goesm-dev/goesm/pull/73)

## v0.0.1-beta.2

Tagged on 2026-10-05. Changes since v0.0.1-beta.1: [v0.0.1-beta.1...v0.0.1-beta.2](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.1...v0.0.1-beta.2).

Go code calls JavaScript and TypeScript with the new `//goesm:import` directive.

### New features

- A function declared without a body, or a package variable without a value, imports an export of an ES module. [#58](https://github.com/goesm-dev/goesm/pull/58)

  ```go
  //goesm:import "./format.ts" formatPrice
  func formatPrice(yen int, currency string) string
  ```

  - Arguments and results are converted according to the Go types: strings, numbers, slices, maps, structs named as encoding/json names their fields, functions, `js.Value` and `any`.
  - A final `error` result receives a thrown exception or a rejection, and `await` makes a function that returns a Promise a blocking Go function.
  - A call costs a few nanoseconds more than the same call from JavaScript, and 3 to 160 times less than through syscall/js.
  - See [docs/js-imports.md](docs/js-imports.md).
- A function imported without `await` that returns a Promise panics with a message naming the directive, instead of converting the Promise to a wrong value. A thrown string, null or number becomes an `Error`. [#67](https://github.com/goesm-dev/goesm/pull/67)
- `goesm build` keeps `node:`, `bun:` and `cloudflare:` imports external, and reports a module that `//goesm:import` names but that cannot be found as the user's error. [#67](https://github.com/goesm-dev/goesm/pull/67)

### Performance

- 64-bit bitwise operations on int64 and uint64 values that are BigInts, such as struct fields, run on machine words in V8. Rand64 goes from 137 to 51 ms on Node.js. [#64](https://github.com/goesm-dev/goesm/pull/64)
- `iter.Pull` switches between coroutines by resolving Promises instead of channel operations: from about 1.7 to 0.75 µs per value on Node.js. [#65](https://github.com/goesm-dev/goesm/pull/65)
- Conversion of non-ASCII strings between UTF-8 and UTF-16 is faster on Node.js and several times faster on Bun. [#58](https://github.com/goesm-dev/goesm/pull/58)
- `json.Marshal` by a generated encoder checks its text with the UTF-8 it encodes instead of a regular expression scan. [#66](https://github.com/goesm-dev/goesm/pull/66)

## v0.0.1-beta.1

Tagged on 2026-10-05. Changes since v0.0.1-beta.0: [v0.0.1-beta.0...v0.0.1-beta.1](https://github.com/goesm-dev/goesm/compare/v0.0.1-beta.0...v0.0.1-beta.1).

This version changes how goesm represents Go values and lowers Go code, so that the output runs at the speed of hand-written JavaScript. On the benchmark at [#53](https://github.com/goesm-dev/goesm/pull/53), goesm's geometric mean is 1.00–1.07× native Go, against 0.91–1.08× for hand-written JavaScript. It also defines what goesm supports by use case, serves HTTP, and rebuilds incrementally.

### New features

- [docs/use-cases.md](docs/use-cases.md) lists the typical code that works on Node.js, at the edge, in the browser and as a library imported from JavaScript or TypeScript, each row checked by a test or by hand. [#49](https://github.com/goesm-dev/goesm/pull/49)
  - `http.ListenAndServe` serves through `node:http`, `Bun.serve` or `Deno.serve`, and `rt.fetchHandler` turns an `http.Handler` into a fetch handler for Cloudflare Workers.
  - The net/http client follows redirects outside browsers.
  - Generated files start with `// @ts-nocheck`, so strict projects such as Next.js and Vite accept them as they are. Callers still see the Go API's types.
- Incremental builds: goesm caches the TypeScript module of every package it lowers, in `goesm/modules` under the user cache directory or in `$GOESMCACHE`, and lowers only the packages that changed. `GOESMCACHE=off` disables the cache. [#54](https://github.com/goesm-dev/goesm/pull/54)
- [docs/concurrency.md](docs/concurrency.md) describes the concurrency model: goroutines run on one thread without preemption. `runtime.Gosched` now also yields to the host's timers and I/O, so a loop that polls with it works. [#57](https://github.com/goesm-dev/goesm/pull/57) [#59](https://github.com/goesm-dev/goesm/pull/59)
- A Promise returned by a `js.FuncOf` callback is passed to JavaScript as it is. [#51](https://github.com/goesm-dev/goesm/pull/51)

### Representation and performance

- Interface methods are on a prototype per type, so V8 inlines interface calls. [#34](https://github.com/goesm-dev/goesm/pull/34)
- A local `[]bool` is backed by bytes. [#35](https://github.com/goesm-dev/goesm/pull/35) [#44](https://github.com/goesm-dev/goesm/pull/44)
- `slices.Sort`, and so `sort.Ints` and `sort.Strings`, sort integers and strings with the engine's sort. [#36](https://github.com/goesm-dev/goesm/pull/36)
- Bounds checks are left out where a loop index is known to be in range. [#37](https://github.com/goesm-dev/goesm/pull/37) [#44](https://github.com/goesm-dev/goesm/pull/44)
- `strings.Builder` builds a string by concatenation, and `strings.Split` and `Join` use the engine's. [#38](https://github.com/goesm-dev/goesm/pull/38)
- Maps with boolean, integer, string and channel keys use the keys directly in a JavaScript Map. [#39](https://github.com/goesm-dev/goesm/pull/39)
- `fmt.Sprintf` of the common verbs is string concatenation, and a call with a constant format is lowered to a concatenation at compile time. [#40](https://github.com/goesm-dev/goesm/pull/40) [#42](https://github.com/goesm-dev/goesm/pull/42) [#50](https://github.com/goesm-dev/goesm/pull/50)
- encoding/json encodes and decodes plain values in one pass, and statically typed `json.Marshal` calls get generated encoders. [#41](https://github.com/goesm-dev/goesm/pull/41) [#42](https://github.com/goesm-dev/goesm/pull/42) [#52](https://github.com/goesm-dev/goesm/pull/52) [#60](https://github.com/goesm-dev/goesm/pull/60)
- A hot int64 or uint64 local is held as two int32 halves instead of a BigInt. `GOESM_SPLIT64=off` turns this off. [#50](https://github.com/goesm-dev/goesm/pull/50)
- ASCII strings that cross the JavaScript boundary are not scanned again, and non-ASCII strings convert faster. [#45](https://github.com/goesm-dev/goesm/pull/45) [#56](https://github.com/goesm-dev/goesm/pull/56)
- Channel operations that complete at once are not awaited, and math/big's word arithmetic is faster. [#62](https://github.com/goesm-dev/goesm/pull/62)
- Code that cannot block stays synchronous next to `iter.Pull` and calls that may block. [#61](https://github.com/goesm-dev/goesm/pull/61)
- bench/ adds cliff kernels, which exercise what Go does fast and a JavaScript translation may not, and a table of each implementation's slowest kernel. [#63](https://github.com/goesm-dev/goesm/pull/63)

### Bundle size

- Package variables whose initializers have no side effects and types that nothing uses are dropped by bundlers. A library that uses only `strings.ToUpper` goes from 93 to 27 KB gzip, and fmt's hello world from 222 to 132 KB gzip. [#43](https://github.com/goesm-dev/goesm/pull/43)

### Fixes

- regexp patterns anchored with `^` and followed by a quantifier, such as `^ab*`, matched nothing. goldmark's GFM task lists work now. [#33](https://github.com/goesm-dev/goesm/pull/33)
- Overwriting an entry of a map with hashed keys keeps the new key, as gc does. [#39](https://github.com/goesm-dev/goesm/pull/39)
- Identifiers named like TypeScript primitives or JavaScript globals, and calls through indexed function values, compile correctly. [#49](https://github.com/goesm-dev/goesm/pull/49)
- Converting a long run of non-ASCII text could throw a RangeError. [#56](https://github.com/goesm-dev/goesm/pull/56)

## v0.0.1-beta.0

Tagged on 2026-10-05. The first tagged version, with every change up to [#32](https://github.com/goesm-dev/goesm/pull/32): [commits](https://github.com/goesm-dev/goesm/commits/v0.0.1-beta.0).

goesm compiles Go packages into native ES modules. The output is TypeScript, one module per Go package, which a bundler, Node.js, Bun or a browser imports as it is. It does not use WebAssembly. This first beta compiles most of the Go language and a large part of the standard library from Go's own source, and its results are checked against native Go.

### Commands

- `goesm emit-ts` writes the TypeScript tree for the host's bundler, and `goesm build` bundles it into an ES module with the embedded esbuild. `-split` writes one module per Go package, and source maps point at the `.go` files. [#1](https://github.com/goesm-dev/goesm/pull/1)
- `-overlay` takes the go command's overlay format, and with `//line` directives, type errors and source maps point at the host file. gosfc uses this for Go blocks in Vue components. [#3](https://github.com/goesm-dev/goesm/pull/3)
- `-toolexec`, also read from `GOFLAGS`, runs tools such as otelc. [#12](https://github.com/goesm-dev/goesm/pull/12)
- `goesm version` prints the module version and the Go version goesm was built with. goesm is installed with `go get -tool` or `go install`; there are no binaries or npm packages. [#4](https://github.com/goesm-dev/goesm/pull/4)

### Language and runtime

- `main` packages run as native Go does under Node.js and Bun: standard output and error, the exit status, Go's `panic:` report with exit status 2, and the deadlock error. `print` and `println` write to standard error in the format of Go's runtime. [#2](https://github.com/goesm-dev/goesm/pull/2) [#6](https://github.com/goesm-dev/goesm/pull/6)
- int64 and uint64 are BigInts and wrap as in Go. int, uint and uintptr stay numbers. Exported functions take and return `bigint` for int64 and uint64. [#7](https://github.com/goesm-dev/goesm/pull/7)
- complex64 and complex128, `new(expr)`, backward `goto`, blocking operations and `defer` and `goto` in range-over-func bodies, local types in generic functions, and `iter.Pull` and `Pull2` work. [#5](https://github.com/goesm-dev/goesm/pull/5) [#10](https://github.com/goesm-dev/goesm/pull/10) [#16](https://github.com/goesm-dev/goesm/pull/16)
- `recover` recovers only when the deferred function calls it directly, as in Go. [#10](https://github.com/goesm-dev/goesm/pull/10)
- Goroutines are async functions on one thread. `sync.Mutex.Lock` is synchronous, so code that only locks does not become async. [#6](https://github.com/goesm-dev/goesm/pull/6)
- `//go:embed`, `//go:linkname` outside the standard library, goroutine-local storage and pointer arithmetic with `unsafe` on field offsets work. [#12](https://github.com/goesm-dev/goesm/pull/12) [#13](https://github.com/goesm-dev/goesm/pull/13)
- A function goesm cannot lower yet becomes a stub that panics when called, instead of failing the build. `goesm build -v` lists the stubs. [#12](https://github.com/goesm-dev/goesm/pull/12)

### Standard library

- reflect is replaced, so fmt, encoding/json and reflect compile from Go's source and give the same results as native Go. [#8](https://github.com/goesm-dev/goesm/pull/8)
- time's timers run on the host's timers. [#9](https://github.com/goesm-dev/goesm/pull/9)
- math/big is exact, so RSA, ECDSA, ECDH, Ed25519 and x509 give the same results as native Go. [#16](https://github.com/goesm-dev/goesm/pull/16)
- crypto in pure Go, crypto/rand, hash/maphash, unique and log/slog work. os uses `node:fs` under Node.js and Bun, os/signal uses `process.on`, and the net/http client uses fetch. [#6](https://github.com/goesm-dev/goesm/pull/6) [#10](https://github.com/goesm-dev/goesm/pull/10) [#12](https://github.com/goesm-dev/goesm/pull/12)
- Programs instrumented by otelc run, and export OTLP/HTTP from Node.js, Bun and browsers. See [docs/otelc.md](docs/otelc.md). [#12](https://github.com/goesm-dev/goesm/pull/12) [#13](https://github.com/goesm-dev/goesm/pull/13)

### Performance

- Struct fields are declared on classes, slice indexing is inline, a `[]byte` is backed by a Uint8Array, interface calls go through the dynamic type's method table at each call site, and numbers are formatted by the engine. [#14](https://github.com/goesm-dev/goesm/pull/14) [#15](https://github.com/goesm-dev/goesm/pull/15) [#17](https://github.com/goesm-dev/goesm/pull/17) [#20](https://github.com/goesm-dev/goesm/pull/20) [#21](https://github.com/goesm-dev/goesm/pull/21) [#23](https://github.com/goesm-dev/goesm/pull/23) [#25](https://github.com/goesm-dev/goesm/pull/25) [#26](https://github.com/goesm-dev/goesm/pull/26) [#32](https://github.com/goesm-dev/goesm/pull/32)
- On the benchmark at [#31](https://github.com/goesm-dev/goesm/pull/31), goesm's geometric mean is 2.6–3.8× native Go, against 2.7–3.4× for Go's WebAssembly port. [#31](https://github.com/goesm-dev/goesm/pull/31)

### Conformance

- Go's own `$GOROOT/test` suite runs through goesm, and 898 of its 961 tests pass. See [docs/conformance.md](docs/conformance.md). [#2](https://github.com/goesm-dev/goesm/pull/2) [#5](https://github.com/goesm-dev/goesm/pull/5) [#10](https://github.com/goesm-dev/goesm/pull/10) [#16](https://github.com/goesm-dev/goesm/pull/16)

### Documentation and tooling

- The README compares goesm with GopherJS, Go's WebAssembly port and TinyGo, and ARCHITECTURE.md describes the compiler. [#1](https://github.com/goesm-dev/goesm/pull/1) [#11](https://github.com/goesm-dev/goesm/pull/11) [#18](https://github.com/goesm-dev/goesm/pull/18) [#19](https://github.com/goesm-dev/goesm/pull/19)
- goesm is licensed under BSD-3-Clause, and `mise.toml` pins the Go, Node.js and Bun versions that development uses. [#4](https://github.com/goesm-dev/goesm/pull/4)
