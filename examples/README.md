# goesm examples

[日本語](README.ja.md)

Each directory is an ordinary Go package in the module `example.com/examples`, with an `index.mjs` that imports the ES module goesm builds from it, and the output `index.mjs` prints (`output.txt`; checked by `TestExamples`, under Node and, when installed, Bun).

| example | shows |
| --- | --- |
| [cart](cart) | the README's shopping cart: structs, slices, `strconv` / `strings.Builder` |
| [textstats](textstats) | the standard library compiled from Go source: `strings`, `strconv`, `sort`, `unicode`, `errors` (`Is`, `Join`), a `(value, error)` result |
| [workers](workers) | goroutines, channels, `sync.WaitGroup` and `sync.Mutex`; blocking functions return Promises |

## Running

```sh
# from the repository root
go build -o bin/goesm ./cmd/goesm
cd examples
../bin/goesm build -o cart/dist ./cart
node cart/index.mjs        # or: bun cart/index.mjs
```

`goesm build` prints how many standard library functions are still stubs (functions goesm cannot lower yet; they panic if called). None of them is reached by these examples. `-v` lists them.

## Calling the Go code

`index.mjs` calls the exported Go functions with ordinary JavaScript values ([docs/js-exports.md](../docs/js-exports.md)):

* Strings are JS strings, slices are arrays, and structs are plain objects: `cart.Total([{ Name: "apple", Price: 120, Quantity: 3 }])`.
* Several results come back as an array, and a final `error` result is thrown as a `GoError`, which is the Go error again when passed back (`ts.IsEmpty(err)`).
* Functions that may block (channel operations, mutex waits) are `async` and return a Promise.
