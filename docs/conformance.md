# Go conformance suite

[日本語](conformance.ja.md)

Go is the authority on Go semantics, so goesm is judged by Go's own tests. `TestGoConformance` (`test/conformance_test.go`) takes the `// run` tests from the Go distribution's `test/` directory, builds each one with goesm, runs the ES module in Node.js and checks it the way Go's own test runner (`cmd/internal/testdir`) checks gc: the program must exit successfully, and its combined stdout and stderr must equal the `.out` file next to the test, or be empty when there is none. GopherJS validates itself the same way.

Node, Bun or browser test suites are not used: they test the JS engine, not goesm. The JS engine is only where the generated ESM runs.

## Running it

```sh
GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
```

It needs Node.js 22+ and the `test/` directory of a full Go distribution (the one from go.dev/dl or `actions/setup-go`). Toolchains downloaded through `GOTOOLCHAIN` ship without it; then point `GOESM_GOROOT_TEST` at one:

```sh
curl -sSL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz | tar xz -C /tmp
GOTOOLCHAIN=go1.27.0 GOESM_CONFORMANCE=1 GOESM_GOROOT_TEST=/tmp/go/test \
  go test ./test -run TestGoConformance -v
```

| variable | meaning |
|---|---|
| `GOESM_CONFORMANCE=1` | enables the suite (it builds about 1000 programs, about 5 minutes on 4 cores) |
| `GOESM_GOROOT_TEST` | the test directory (default `$(go env GOROOT)/test`) |
| `GOESM_CONFORMANCE_DIRS` | comma-separated subdirectories (default `.,ken,chan,interface,typeparam,fixedbugs`) |
| `GOESM_CONFORMANCE_RUN` | regexp on test names, e.g. `^ken/` or `typeswitch` |
| `GOESM_CONFORMANCE_NATIVE=1` | also runs every test with native `go run` and leaves out tests whose native output differs from the `.out` file (a check on the harness itself) |
| `GOESM_CONFORMANCE_OUT` | writes per-test results as TSV (status, imports, Node run time, reason) |
| `GOESM_CONFORMANCE_UPDATE=1` | rewrites the baseline |

A test that times out (20 s in Node, 2 min for the build) is run again on its own after the parallel run, so a busy CI runner does not turn a slow start into a failure; it fails if it still times out alone.

Each test is copied into its own one-package module (`go` directive = the running toolchain), built with `goesm build`, and run by a small driver that imports the bundle and exits as soon as `main` returns, like a Go program, with status 2 on an unrecovered panic.

Only tests whose recipe is a bare `// run` are selected. Tests with arguments or go command flags (`// run -gcflags=...`), multi-file `rundir` tests and compiler-only recipes (`errorcheck`, `compile`, `asmcheck`) are not. Tests whose build constraints exclude `js/wasm` (goesm's target) are reported as skipped.

## Baseline

`test/conformance/passing.txt` lists the tests that pass. A listed test that fails is a regression and fails the suite; a test that passes but is not listed is reported so the list can be updated with `GOESM_CONFORMANCE_UPDATE=1`. CI runs the suite as a separate `conformance` job.

## Results

Go 1.27.0 `test/` directory, Node.js 22, goesm at main of 2026-10-04 (after the second round of fixes for this suite):

| directory | pass rate | skipped |
|---|---|---|
| `test/` | 86.8% (118/136) | 9 |
| `chan/` | 100.0% (17/17) | 0 |
| `fixedbugs/` | 91.4% (563/616) | 30 |
| `interface/` | 100.0% (11/11) | 0 |
| `ken/` | 100.0% (40/40) | 0 |
| `typeparam/` | 98.6% (139/141) | 0 |
| **total** | **92.4% (888/961)** | 39 |
| tests without imports | 98.8% (506/512) | |

With `GOESM_CONFORMANCE_NATIVE=1`, native `go run` reproduces the `.out` file for every selected test except 11 that shell out to the go command (`os/exec`), which goesm cannot build anyway.

Tests that import a standard library package, by package (a test counts once for each package it imports):

| package | pass rate |
|---|---|
| `fmt` | 89.5% (187/209) |
| `runtime` | 67.3% (74/110) |
| `reflect` | 76.5% (52/68) |
| `os` | 86.2% (50/58) |
| `unsafe` | 56.4% (31/55) |
| `strings` | 66.7% (28/42) |
| `math` | 96.6% (28/29) |
| `time` | 100.0% (19/19) |
| `strconv` | 87.5% (14/16) |
| `sync` | 100.0% (15/15) |

The suite prints this table for every run. The 73 failures fall into these groups:

| group | tests |
|---|---|
| no address space: `unsafe` pointer arithmetic, `uintptr` to pointer conversions, reinterpreting memory through `unsafe.Pointer` | 23 tests such as `cmp`, `strcopy`, `unsafebuiltins`, `fixedbugs/issue15329` (reported by goesm at build time) |
| `runtime.Caller`, stack traces and PC tables | `inline_literal`, `devirtualization_nil_panics`, `fixedbugs/bug347`, `issue4562`, `issue5856`, `issue7690`, `issue14646`, `issue18149`, `issue21879`, `issue22083`, `issue22662`, `issue27201`, `issue29504`, `issue33724`, `issue56990`, `issue58300`, `issue58300b`, `issue79762` |
| garbage collector observations (finalizers, `MemStats`, liveness) | `init1`, `stackobj`, `stackobj3`, `fixedbugs/issue15281`, `issue27518b`, `issue32477`, `issue46725`, `issue54343` |
| not supported yet | `range4` and `fixedbugs/issue71675` (`defer` in a range-over-func body), `fixedbugs/issue72063` and `typeparam/nested` (local types depending on type parameters), `fixedbugs/issue30606`, `issue30606b`, `issue49110` (`reflect.StructOf`), `fixedbugs/issue73748a`, `issue73748b` (`runtime/trace`) |
| `recover` through a function value wrapping a method (known difference) | `fixedbugs/issue73917`, `issue73920`; `recover` and `recover1` also test recursive and reflect-made deferred calls |
| 64-bit `int` (a JS number, exact below 2^53) | `divmod` (times out), `fixedbugs/issue30116u` |
| addresses and memory layout | `nilptr`, `fixedbugs/bug260`, `bug348`, `issue29190` (a slice of zero-size elements longer than a JS array can be) |
| resources | `fixedbugs/issue34395` (a 100 MiB array literal: goesm needs over 4 GB to build it), `issue25897a`, `issue30977`, `issue78081` (no exit within 20 s) |
| environment | `winbatch` (reads `src/all.bat` from GOROOT) |
