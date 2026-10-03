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

Each test is copied into its own one-package module (`go` directive = the running toolchain), built with `goesm build`, and run by a small driver that imports the bundle and exits as soon as `main` returns, like a Go program, with status 2 on an unrecovered panic.

Only tests whose recipe is a bare `// run` are selected. Tests with arguments or go command flags (`// run -gcflags=...`), multi-file `rundir` tests and compiler-only recipes (`errorcheck`, `compile`, `asmcheck`) are not. Tests whose build constraints exclude `js/wasm` (goesm's target) are reported as skipped.

## Baseline

`test/conformance/passing.txt` lists the tests that pass. A listed test that fails is a regression and fails the suite; a test that passes but is not listed is reported so the list can be updated with `GOESM_CONFORMANCE_UPDATE=1`. CI runs the suite as a separate `conformance` job.

## Results

Go 1.27.0 `test/` directory, Node.js 22:

| directory | pass rate | skipped |
|---|---|---|
| `test/` | 32.4% (44/136) | 9 |
| `chan/` | 41.2% (7/17) | 0 |
| `fixedbugs/` | 50.6% (312/616) | 30 |
| `interface/` | 72.7% (8/11) | 0 |
| `ken/` | 70.0% (28/40) | 0 |
| `typeparam/` | 41.8% (59/141) | 0 |
| **total** | **47.7% (458/961)** | 39 |
| tests without imports | 87.5% (448/512) | |

With `GOESM_CONFORMANCE_NATIVE=1`, native `go run` reproduces the `.out` file for every selected test except 11 that shell out to the go command (`os/exec`), which goesm cannot build anyway.

Most failures are tests that import the standard library (`fmt` alone accounts for 107; then `runtime`, `reflect`, `os`, `math`), which goesm cannot compile yet (ARCHITECTURE.md §9). Among the 512 tests without imports, the 64 failures fall into these groups:

| group | tests |
|---|---|
| not supported yet: complex numbers | `convT2X`, `print`, `ken/cplx0`, `cplx1`, `cplx2`, `cplx5`, `fixedbugs/bug329`, `bug401`, `bug491`, `issue5793`, `issue58671`, `issue79812` |
| not supported yet: `goto` | `ken/label`, `fixedbugs/bug005`, `bug178`, `issue13684`, `issue40367`, `issue4748`, `issue75569` |
| not supported yet: other | `convert4` (slice to array pointer), `range4` and `fixedbugs/issue71675` (labeled branches in range-over-func), `typeparam/issue54537` (address of a type-parameter variable) |
| 64-bit integers (known difference) | `intcvt`, `printbig`, `divmod` (times out), `fixedbugs/issue2615`, `issue4448`, `issue43480`, `issue50854`, `issue70481`, `issue23305` |
| indirect `recover` (known difference) | `fixedbugs/issue73916`, `issue73916b`, `issue73917`, `issue73920` |
| package variables initialised from a multi-value expression (`var a, ok = m[k]`, `var x, y = f()`, `var v, ok = i.(T)`) | `fixedbugs/bug227`, `bug244`, `bug291`, `issue53619` |
| goesm crashes (stack overflow) on recursive types (`type S []S`, `type Chan[T any] chan Chan[T]`) | `ddd`, `fixedbugs/issue17039`, `typeparam/issue47901` |
| method calls on type parameters / generic method sets (`reading 'methods'`, ARCHITECTURE known gap) | `typeparam/issue49421`, `issue53419`, `issue54225`, `shape1`, `typeswitch3`, `fixedbugs/issue54348` |
| other generics bugs | `typeparam/interfacearg` (internal error), `issue50833` (composite literal of type `P`), `issue44688`, `issue376214` |
| method expressions (promoted methods, literal receiver types) | `method`, `method7` |
| `new(expr)` (Go 1.26) is lowered as `new(T)` | `newexpr` |
| `defer x.M()` with a nil interface must panic at the defer statement | `fixedbugs/issue15975` |
| nil dereference that should panic (`&*p`, method value of a nil pointer) | `nilptr2`, `method5` |
| zero-size values (`[0]int`, `struct{}`) | `zerosize`, `fixedbugs/bug352` |
| range assignment order: in `for i, x[i] = range y`, `x[i]` must use the old `i` | `range` |
| too slow (no exit within 20 s) | `fixedbugs/issue13169` (100k channel sends), `issue34395` (100 MiB array literal) |
