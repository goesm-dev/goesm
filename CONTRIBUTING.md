# Contributing to goesm

[日本語](CONTRIBUTING.ja.md)

Thanks for helping. goesm is experimental, so issues that show a Go program behaving differently under goesm than under native Go are as welcome as code.

## Principles

* **Go is the authority.** Parsing, module resolution and type checking come from the Go toolchain (go/packages + go/types). goesm never adds its own syntax, module system or type system; if `go vet` accepts a package, that is the input goesm has to handle.
* **Native Go is the reference.** A behaviour is correct when it matches `go run`. Known differences are listed in [ARCHITECTURE.md](ARCHITECTURE.md) §11 and pinned by `TestKnownGaps`.
* **No compiler plugins.** Importing a dependency never runs code inside goesm. The standard library replacements and natives are a fixed set inside goesm that applies only to `$GOROOT/src` (ARCHITECTURE.md §10).
* **Output is plain ESM.** The emitted TypeScript tree must work in any bundler or TypeScript-aware runtime without a plugin, and must type-check under strict `tsc`. Browser ESM is the main target; Node.js and Bun must both work, so the runtime uses no Node-only API.
* **Performance matters.** goesm exists as a faster alternative to WebAssembly, so weigh the speed of the generated JavaScript when choosing a representation.

## Setup

[mise](https://mise.jdx.dev) installs the pinned versions of Go, Node.js and Bun from `mise.toml`, the same ones CI uses:

```sh
mise install
npm ci --prefix test   # tsc, oxlint and workerd, pinned in test/package.json
```

Without mise you need Go 1.27+ and Node.js 22.18+; Bun is optional. When you change a tool version, pin an exact version (never `latest`) in `mise.toml` or `test/package.json`.

## Tests

```sh
go test ./...
```

| test | checks |
| --- | --- |
| `TestGolden` | every parameterless exported function in `testdata/semantics/{basics,generics,goroutines,panics,stdlibuse}` returns the same value under native Go and in the ESM goesm builds |
| `TestJS` | `test/js/*.test.mjs` (node:test) against built bundles |
| `TestKnownGaps` | the documented differences from native Go still exist (`testdata/semantics/gaps`) |
| `TestExamples` | `examples/*` run under Node.js, and Bun when it is installed, and print their `output.txt` |
| `TestTSC` | the emitted TypeScript type-checks with strict `tsc` once its `@ts-nocheck` header is removed, and TypeScript callers see the Go types with the header in place |
| `TestOxlint` | the built ESM has no oxlint correctness findings |
| `TestPrograms` | the commands in `testdata/programs` print the same output and exit with the same status under native Go and goesm (Node.js and Bun) |
| `TestToolexec` | a `-toolexec` program's rewrites of a module and of the standard library reach goesm's output as they reach `go build`'s (`testdata/toolexec`) |
| `TestFetch` | HTTP clients use `fetch`, also through a Transport with a plain `net.Dialer`, and a custom dialer is still called (`testdata/fetch`, against a local server) |
| `TestUseCase*` | the use cases of [docs/use-cases.md](docs/use-cases.md) (`testdata/usecases`) behave as natively: command-line tools, a build tool, SSR, popular libraries, an HTTP and Connect server under `http.ListenAndServe` (Node.js and Bun) and as a Cloudflare Workers fetch handler (workerd) |
| `TestStdlibStatus -v` | reports which standard library packages lower and how many functions are stubs |

`TestTSC` and `TestOxlint` skip when `npm ci --prefix test` has not been run; CI sets `GOESM_REQUIRE_TOOLS=1` so they cannot be skipped there. CI also checks `gofmt -l .` and `go vet ./...`.

### otelc

`TestOtelc` instruments `testdata/otelc` with otelc for the host and for goesm and compares the telemetry ([docs/otelc.md](docs/otelc.md)). It runs when `GOESM_TEST_OTELC` names an otelc binary, and needs network access for the modules `otelc setup` adds:

```sh
GOBIN=/tmp/otelc go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.1.0
GOESM_TEST_OTELC=/tmp/otelc/otelc go test ./test -run TestOtelc -v
```

### Go conformance suite

`TestGoConformance` builds the `// run` tests of the Go distribution's `test/` directory with goesm and compares their output with the `.out` files, as Go's own test runner does. It is opt-in because it builds about 1000 programs:

```sh
GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
GOESM_CONFORMANCE=1 GOESM_CONFORMANCE_RUN='^ken/' go test ./test -run TestGoConformance -v   # a subset
```

It needs the `test/` directory of a full Go distribution; the Go that `mise install` sets up has it, toolchains downloaded through `GOTOOLCHAIN` do not. To reproduce one failing case, copy it into a fresh module and compare with native Go:

```sh
mkdir /tmp/case && cp "$(go env GOROOT)/test/ken/chan.go" /tmp/case/main.go
cd /tmp/case && go mod init case && go run . > native.txt 2>&1
goesm build ./ && node dist/case.js > goesm.txt 2>&1
diff native.txt goesm.txt
```

See [docs/conformance.md](docs/conformance.md) for the variables, the baseline in `test/conformance/passing.txt` and how to update it.

## Pull requests

* **Every semantic fix comes with a golden fixture**: add a parameterless exported function reproducing the bug to a package under `testdata/semantics/`, so `TestGolden` compares it with native Go. If the fix makes a conformance test pass, update the baseline too.
* If goesm still differs from native Go on purpose, add the case to `testdata/semantics/gaps` and to ARCHITECTURE.md §11 instead.
* Keep generated code passing `TestTSC` and `TestOxlint`; do not turn rules off to get green.
* Documentation has an English file and a Japanese `.ja.md` next to it; update both.
* Run `gofmt` and `go test ./...` before pushing.

## License

goesm is released under the [BSD 3-Clause License](LICENSE). By contributing, you agree that your contributions are licensed under it.
