# Use cases

[日本語](use-cases.ja.md)

goesm's support is defined by use case, not by language feature. The policy:

- **Supported:** the typical code of each use case below (roughly the 75th percentile of what people write for it) works as it does natively. Each row says how it is checked; most are compared with native Go in CI.
- **Not supported yet:** less common code (up to roughly the 95th percentile) may work, but nothing guarantees it. The [list](#not-supported-yet) says what is known to be missing.
- **Out of scope:** what a JavaScript host cannot provide (cgo, raw memory, the gc runtime's internals).

The use cases are grouped by where the code runs, Node.js, the edge and the browser, plus two that span all three: a Go library imported from JavaScript or TypeScript, and JavaScript or TypeScript called from Go.

## Node.js: command-line tools, servers, SSR and build tools

Node.js here also means Bun, and for servers Deno. Files, environment variables, standard input and output, signals and goroutines waiting on each other all work.

| Use case | Typical code | Status | Checked by |
| --- | --- | --- | --- |
| Command-line tool | `flag`, `os` (arguments, environment, exit status, files), `path/filepath` (`WalkDir`, `Glob`), `bufio.Scanner` on standard input, `encoding/csv`, `encoding/json`, `regexp`, `text/template`, `log`, `log/slog` | Supported | `TestUseCaseCLI` (`testdata/usecases/cli`), Node.js and Bun |
| Command-line tool with cobra | `spf13/cobra` subcommands, flags, help, errors, shell completion | Supported | `TestUseCaseCLI` (`testdata/usecases/cobra`) |
| Build tool | Markdown with `yuin/goldmark`, `gopkg.in/yaml.v3`, `BurntSushi/toml`, `compress/gzip`, `crypto/sha256`, reading and writing directories | Supported | `TestUseCaseBuildTool` (`testdata/usecases/site`); [goesm.dev](https://github.com/goesm-dev/goesm.dev) builds its own site this way |
| Server-side rendering | `html/template` with `embed`, called from the JS framework's server side | Supported | `TestUseCaseBuildTool` (`testdata/usecases/ssr`); Next.js Server Components by hand |
| HTTP server | `http.ListenAndServe` with a `ServeMux` (method and path patterns), JSON, forms, cookies, redirects, middleware, `context`, Server-Sent Events, graceful `Shutdown` on a signal | Supported | `TestUseCaseServer` (`testdata/usecases/server`), Node.js and Bun; Deno by hand |
| Connect server | `connectrpc.com/connect` handlers with the Connect, Connect JSON and gRPC-Web protocols, unary and server streaming, errors and headers | Supported | `TestUseCaseServer` (`testdata/usecases/greet`) |
| HTTP and Connect client | `net/http`'s client (it uses `fetch`), redirects followed by `http.Client` as natively, `connectrpc.com/connect` clients | Supported | `TestUseCaseServer`, `TestFetch` |

`http.ListenAndServe` starts the host's own server: `node:http` under Node.js, `Bun.serve` and `Deno.serve`. Each request runs in its own goroutine.

## Edge: Cloudflare Workers and other fetch-handler runtimes

An `http.Handler` becomes the Worker's fetch handler:

```ts
import { Handler } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: Handler() };
```

| Use case | Typical code | Status | Checked by |
| --- | --- | --- | --- |
| HTTP API on Workers | the same `http.Handler` as on Node.js: `ServeMux`, JSON, cookies, redirects, `context` | Supported | `TestUseCaseEdge` in workerd, Cloudflare's Workers runtime (`testdata/usecases/edge`) |
| Connect server on Workers | Connect, Connect JSON and gRPC-Web, unary and server streaming | Supported | `TestUseCaseEdge` |
| Configuration and secrets | `os.Getenv` of the Worker's text bindings and secrets, with the `nodejs_compat` flag | Supported | `TestUseCaseEdge` |
| Outbound requests, crypto, streaming | `http.Get` and Connect clients over `fetch`, `crypto/hmac`, `crypto/sha256`, `crypto/rand`, flushing a response (Server-Sent Events) | Supported | `TestUseCaseEdge` |
| Next.js edge Route Handler | a Route Handler with `export const runtime = "edge"` that calls Go | Supported | by hand |

`fetchHandler` also works with `Deno.serve`, `Bun.serve` and service workers. The request body is read in full before the handler runs; the response is sent when the handler returns, or streams from the handler's first flush.

## Browser (CSR)

| Use case | Typical code | Status | Checked by |
| --- | --- | --- | --- |
| Domain logic called from a component | structs, methods, errors (`errors.Is`, `As`, `Join`), generics, `encoding/json`, `regexp`, `strings`, `strconv`, `time` | Supported | by hand in Chromium, with Vite |
| DOM from Go | `syscall/js`: creating and finding elements, event listeners through `js.FuncOf`, reading inputs, timers and goroutines | Supported | by hand in Chromium |
| Connect client in the browser | Connect, Connect JSON and gRPC-Web, unary and server streaming (incremental), deadlines and errors | Supported | by hand in Chromium, with Vite |
| React, Preact and Next.js | Go functions called from TSX: in render, event handlers and effects, Next.js Client and Server Components and Route Handlers, with Turbopack and webpack | Supported | by hand in Chromium (React 19 and Preact with Vite 8, Next.js 16) |

The output is ES modules (TypeScript), so Vite, Rolldown, Turbopack, webpack and esbuild bundle it with no plugin; Go is not written inside JSX but called from it like any other module. Every generated file starts with `// @ts-nocheck`, so a project's own strictness flags (`noUnusedLocals`, an ES2017 target) do not re-check generated code, while the exported types still reach the caller. Two settings remain: TypeScript needs `allowImportingTsExtensions` to import a module by its `.ts` name (or import it without the extension), and a goesm tree shipped as a package in `node_modules` needs Next.js's `transpilePackages`.

What the Go code adds to a page's JavaScript, measured with Vite 8 (minified, gzip):

| What the page uses | gzip |
| --- | ---: |
| A package that only uses `strings` | 11 KiB |
| `syscall/js` DOM code | 43 KiB |
| `fmt` (hello world) | 117 KiB |
| Domain logic with `encoding/json` (which brings `fmt` and `reflect`) | 276 KiB |
| A Connect client (protobuf, `net/http`) | 1.3 MiB |

## Go libraries used from JavaScript and TypeScript

Every exported function of the Go package you build is an export of its module, typed in TypeScript, in any of the places above.

| Use case | Typical code | Status | Checked by |
| --- | --- | --- | --- |
| Calling exported Go functions from TS | functions taking and returning strings, arrays and plain objects, handles with methods, multiple results as arrays, `error` thrown, functions that block returning Promises | Supported | `TestTSC`, `TestJS`, `TestExamples` |
| Popular pure-Go libraries | `google/uuid`, `golang.org/x/mod/semver`, `Masterminds/semver`, `shopspring/decimal`, `go-playground/validator`, `expr-lang/expr`, `tidwall/gjson`, `golang.org/x/text`, `yuin/goldmark`, `gopkg.in/yaml.v3` | Supported | `TestUseCaseLibraries` (`testdata/usecases/libs`) and `TestUseCaseBuildTool` |

Arguments and results are converted at the boundary by their Go types: strings, arrays and plain objects are passed and returned as they are in JavaScript. [js-exports.md](js-exports.md) has the details.

## JavaScript and TypeScript used from Go

Go code calls the functions and uses the values of ES modules it declares with `//goesm:import`, in any of the places above. [js-imports.md](js-imports.md) has the details.

| Use case | Typical code | Status | Checked by |
| --- | --- | --- | --- |
| Calling the project's TS and JS functions from Go | calls passing strings, slices, maps, structs and functions, calls awaiting a Promise, exceptions received as errors | Supported | `TestJSImport` (`testdata/jsimport`), Node.js and Bun |
| Classes and values of npm packages | a class received as a `js.Value`, used with `New` and `Call` | Supported | by hand |
| Vue components from Go | a component imported in gosfc's Go block, used in the template | Supported by gosfc | gosfc's tests |

## Not supported yet

These are known gaps between the 75th and 95th percentiles; code that needs them may still work in part.

- **Fields of handles:** a pointer to a struct type with methods reaches JavaScript as the Go object itself, so its fields hold Go's own representation; data is read through methods and functions.
- **Bundle size.** A package using `fmt.Sprintf` is about 95 KiB gzip, most of it `reflect`, which `fmt` uses for every argument. A package variable initialized by a call other than `errors.New` (`regexp.MustCompile`, for one) keeps its package in the bundle even when the importer uses none of it. `net/http`'s client keeps its TLS and HTTP/2 code although requests go through `fetch`. Code that uses only parts of `time`, `strings` or `strconv` stays small: six functions of `time` come to 13 KiB.
- **HTTP servers:** HTTP/2 and gRPC's own protocol (Connect and gRPC-Web work), TLS (`ListenAndServeTLS`), WebSockets and `Hijack`, trailers, request bodies streamed while the handler runs, client and bidirectional streaming RPCs, and `Serve` on a `net.Listener`.
- **Workers beyond `fetch`:** bindings such as KV, D1, R2 and Durable Objects are reachable only through `syscall/js`, and `ctx.waitUntil` is not connected, so goroutines still running after the response may be stopped.
- **Network and processes:** `net.Dial` and `net.Listen` of raw TCP or UDP, database drivers that dial TCP (`pgx`, `go-sql-driver/mysql`), and `os/exec`.
- **Time zones in browsers:** `time.LoadLocation` needs `import _ "time/tzdata"` there, and `time.Local` is a fixed offset taken at start-up, as in Go's own js/wasm port.
- **64-bit `int` and `uint`:** they are exact below 2^53 and do not wrap on overflow; `int64` and `uint64` are exact.
- **Parallelism:** goroutines share one JavaScript thread and switch only where one waits, as in Go with `GOMAXPROCS=1` and without preemption. Go code that computes for long without waiting holds up the other goroutines, timers and requests, and code that polls a flag without `runtime.Gosched` hangs. Parallel work goes in Workers. [docs/concurrency.md](concurrency.md) describes the model, data races and memory safety.
- **Panics:** a panic nobody recovers in a goroutine ends the process, as in Go, also when the Go code is a library inside a Node.js server such as Next.js. A panic in a `js.FuncOf` callback goes to `reportError` instead of the caller.
- **Packaging:** Node.js does not strip types from `.ts` files under `node_modules`, so a goesm tree published as an npm package needs a bundler. Source maps name the `.go` files by absolute path.
- **Tooling:** packages that import `syscall/js` build, vet and load in gopls only with `GOOS=js GOARCH=wasm`.

## Out of scope

- cgo, `plugin`, and assembly outside the standard library.
- `unsafe` beyond what [ARCHITECTURE.md §7](../ARCHITECTURE.md#7-reflect--unsafe--memory-representation) describes: reinterpreting memory between unrelated layouts and addresses that outlive their pointers.
- What only the gc runtime can answer: `runtime.Caller` and `Stack`, finalizers and observing the garbage collector, goroutine preemption.
- WebAssembly output: goesm compiles to JavaScript.
