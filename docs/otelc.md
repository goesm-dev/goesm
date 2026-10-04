# Compile-time instrumentation (otelc)

[otelc](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation), OpenTelemetry's compile-time instrumentation for Go, works with goesm unchanged: the program's HTTP requests, logs and spans are traced in Node.js, Bun and browsers as they are in a native build.

## Usage

`otelc setup` must prepare the module for goesm's target, `GOOS=js GOARCH=wasm`, with the toolchain goesm uses; then `goesm build` (or `emit-ts`) takes otelc as its `-toolexec` program, as `go build` does:

```sh
GOOS=js GOARCH=wasm otelc setup
goesm build -toolexec "otelc toolexec" ./cmd/app
OTEL_TRACES_EXPORTER=console node dist/app.js
```

`-toolexec` in `GOFLAGS` works too (`GOFLAGS=-toolexec='otelc toolexec' goesm build ./cmd/app`). A module set up for the host (`otelc setup` without `GOOS`/`GOARCH`) does not build for js/wasm: keep separate setups, for example in separate checkouts.

## What matches native Go

`TestOtelc` (`go test ./test -run TestOtelc`, with `GOESM_TEST_OTELC` naming an otelc binary) builds [testdata/otelc](../testdata/otelc) natively and with goesm and requires the same telemetry from the console exporter, under Node.js and Bun:

* the spans otelc's `net/http` client instrumentation creates, with their names, kinds, attributes and status;
* a span started with the OpenTelemetry API, and the HTTP spans nested under it, also from a goroutine started inside it (otelc propagates the current span through goroutine-local storage);
* `log` lines correlated with the current span (`trace_id=... span_id=...`), and the SDK's own `slog` output.

The resource describes the host the program runs on: `os.type` is `js`, `process.runtime.name` is `goesm`, and under Node.js and Bun `process.executable.name` is the script.

## How it works

* `-toolexec`: goesm runs the build for its target through otelc and records the source each compile sees (otelc rewrites functions to call hooks and adds files), then lowers that source. See [ARCHITECTURE.md §3](../ARCHITECTURE.md#3-frontend-the-go-toolchain-as-is).
* The hooks are reached through `//go:linkname`, goroutine-local storage is otelc's `runtime` API over goesm's goroutines, and the OpenTelemetry SDK's dependencies (`//go:embed`, `unsafe` header structs in protobuf) lower as described in [ARCHITECTURE.md §7 and §9](../ARCHITECTURE.md#9-standard-library).

## Limitations

* The OTLP exporters do not work yet: they encode with protobuf-go, whose generated-message fast path does pointer arithmetic on struct field offsets, which goesm cannot lower (those functions are stubs, and exporting panics). Use the console exporter, or `OTEL_TRACES_EXPORTER=none`.
* An HTTP server cannot listen in Node.js or Bun (`net.Listen` is the js/wasm port's in-process network), so the server instrumentation only applies to in-process servers, whose clients must not use `fetch`.
* Instrumentation of other libraries (database drivers, gRPC, ...) is untested; a library function goesm cannot lower becomes a stub that panics if called (`goesm build -v` lists them).
