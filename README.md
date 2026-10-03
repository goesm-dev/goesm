# goesm

goesm turns real Go packages into ES modules. The current Go toolchain is the
frontend (go/packages + go/types), goesm lowers Go semantics to TypeScript plus
a small runtime (`@goesm/runtime`), and esbuild's Go API produces the ESM.

```
Go  →  goesm  →  TypeScript  →  esbuild  →  ESM
```

This is a proof of concept. See [ARCHITECTURE.md](ARCHITECTURE.md) for the
design, what is implemented and what is not, and
[docs/gopherjs-comparison.md](docs/gopherjs-comparison.md) for how it differs
from GopherJS. [docs/example-output.md](docs/example-output.md) shows generated
TypeScript and JavaScript.

## Usage

Inputs are ordinary Go package patterns in an ordinary Go module:

```sh
cd testdata/example
go run ../../cmd/goesm build ./main          # dist/main.js (+ .js.map pointing at .go)
go run ../../cmd/goesm build -split ./main   # one ES module per Go package
go run ../../cmd/goesm emit-ts ./main        # inspect the generated TypeScript
```

```js
import { Result } from "./dist/main.js";
Result(); // 3
```

goesm uses go/types from the toolchain it was built with, so build it with the
toolchain your module uses (for example via a `tool` directive and
`go tool goesm`). This repository's fixtures use Go 1.27.

## Tests

```sh
go test ./...   # needs Go 1.27+ and Node.js 22+
```

* `TestGolden` runs every parameterless exported function of the fixtures
  under native Go and in the goesm-built ESM (Node) and requires equal results.
* `TestJS` runs `test/js/*.test.mjs` (node:test) against built bundles.
* `TestKnownGaps` pins the documented differences from native Go.
* `TestStdlibStatus -v` reports how far standard library packages get.
