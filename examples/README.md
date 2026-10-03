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

## What the JS side has to do today

There is no JS calling ABI yet (see ARCHITECTURE.md §7), so `index.mjs` converts values by hand with the runtime each module re-exports as `$runtime`:

* Go strings are byte strings: `rt.fromJSString(s)` in, `rt.toJSString(s)` out.
* Slices: `rt.sliceLit([...])` in, `rt.toArray(s)` out.
* Structs are classes with positional constructors: `new cart.Item(name, price, quantity)`.
* Multiple results come back as an array, an `error` as a Go interface value (`rt.icall(err, "Error")`).
* Functions that may block (channel operations, mutex waits) are `async` and return a Promise.
