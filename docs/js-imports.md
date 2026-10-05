# Calling JavaScript from Go

[日本語](js-imports.ja.md)

Go code compiled by goesm can call functions and use values of any ES module: a TypeScript or JavaScript file of your project, a package from `node_modules`, or a Vue component through [gosfc](https://github.com/goesm-dev/gosfc). The Go side declares what it imports, with the Go types it uses, and goesm converts the values at the boundary.

```ts
// format.ts
export function formatPrice(yen: number, currency: string): string {
  return new Intl.NumberFormat("ja-JP", { style: "currency", currency }).format(yen);
}
```

```go
package shop

//goesm:import "./format.ts" formatPrice
func formatPrice(yen int, currency string) string

func Label(price int) string {
	return "Price: " + formatPrice(price, "JPY")
}
```

## The directive

`//goesm:import` precedes a function declared without a body, or a package variable declared without a value:

```go
//goesm:import "<module>" [<export>] [await]
```

- **module** is an import specifier. A path starting with `./` or `../` is relative to the Go file and must exist; goesm writes it into the generated module relative to where that module is. Anything else, such as `chart.js`, names a package: `goesm build` looks for it in the `node_modules` directories of the directory it runs in and of its parents, and the output of `goesm emit-ts` keeps the specifier for your bundler. A module built into the JavaScript runtime, such as `node:path`, `bun:sqlite` or `cloudflare:sockets`, stays an import in the output of `goesm build` and is resolved where the output runs.
- **export** is the name of the export. `default`, or nothing, imports the default export, and `*` the module namespace object.
- **await** marks a function that returns a Promise. Go calls it as a function that blocks until the Promise settles, like a channel receive, so the functions calling it become asynchronous in JavaScript too. A function that returns a Promise needs `await`: without it, the call panics when the Promise comes back. The Promise is kept only for a `js.Value` result, and is left running for a function without results.

```go
//goesm:import "chart.js" Chart
var Chart js.Value // a class: Chart.New(canvas, config)

//goesm:import "./api.ts" fetchUser await
func fetchUser(id string) (User, error)

//goesm:import "./config.ts" VERSION
var version string
```

A package variable is initialized before the package's other variables, from the value the module exports when the Go package is initialized.

## Conversions

Arguments are converted from Go to JavaScript and results from JavaScript to Go, according to the types of the declaration. The declaration is the only description of the JavaScript function goesm has; nothing is read from TypeScript types.

| Go | JavaScript |
| --- | --- |
| `bool`, integers, floats | boolean, number |
| `int64`, `uint64` | bigint |
| `string` | string |
| `js.Value`, `js.Func` | the value itself |
| `[]byte` | `Uint8Array` |
| other slices and arrays | array |
| `map[string]T` | plain object |
| struct, pointer to struct | plain object with the exported fields, named as `encoding/json` names them (the `json` tag name, or the field name); an embedded struct or pointer to struct contributes its fields |
| function (arguments only) | function, which converts its own arguments and results the same way |
| `any` | to JavaScript, the conversion of the dynamic value; to Go, the value as `encoding/json` decodes it into an `any` |
| variadic parameter | separate arguments |

Values are copied: a slice, map or struct changed on the other side does not change the original. A JavaScript value that should stay itself, such as a DOM element or a class instance, is a `js.Value`. Several results are an array returned by the JavaScript function. Types without a JavaScript counterpart, such as channels and complex numbers, are compile errors at the directive.

## Errors

If the last result is `error`, an exception thrown by the function, or the rejection of its Promise, is returned as that error, with the other results zero; the function returning normally gives a nil error. Without an `error` result, the exception becomes a panic, which `recover` can stop. In both cases the error is a `js.Error` when the Go package imports `syscall/js`, so `errors.As` can reach the thrown value; otherwise it is an error with the same message, `JavaScript error: <message>`. A thrown value that is not an object, such as a string or `null`, is wrapped in an `Error` whose message is that value converted to a string and whose `cause` is the value.

```go
//goesm:import "./lib.ts" parsePrice
func parsePrice(s string) (float64, error)

_, err := parsePrice("abc")
var jerr js.Error
if errors.As(err, &jerr) {
	fmt.Println(jerr.Get("name").String()) // Error
}
```

## Performance

A call through `//goesm:import` costs a few nanoseconds more than a call from JavaScript to the same function when the arguments are numbers, ASCII strings or structs of such fields. The same call through `syscall/js` costs 3 times as much with Japanese text and up to 160 times as much with a struct. [bench/jsimport](../bench/jsimport) measures them:

| Call | JS → JS | Go → JS, `//goesm:import` | Go → JS, `syscall/js` |
| --- | ---: | ---: | ---: |
| `add(int, int) int` | 2.2 ns | 3.0 ns | 163 ns |
| `strlen(string) int`, ASCII | 2.6 ns | 7.8 ns | 146 ns |
| `strlen(string) int`, Japanese | 5.0 ns | 117 ns | 317 ns |
| `upper(string) string`, ASCII | 30 ns | 48 ns | 280 ns |
| `total(struct) int` | 7.3 ns | 5.3 ns | 840 ns |
| `sum([]float64) float64`, 8 elements | 17 ns | 37 ns | 1,406 ns |

Node.js 22 on a 4 vCPU Intel Xeon 2.80 GHz cloud VM. On Bun, calls with numbers and strings cost about the same, and calls with a struct or a slice about twice as much. A string with non-ASCII characters is re-encoded between UTF-8 and UTF-16 each way, which is the remaining cost; slices are copied element by element.

## Limitations

- A package with `//goesm:import` declarations builds only with goesm: `go build` reports the functions without bodies. `go vet` and gopls accept it.
- `goesm build` bundles with esbuild, which handles `.ts`, `.js` and `.mjs` files but not `.vue`; Vue components are used through gosfc, which builds with Vite.
- A JavaScript function returned to Go is a `js.Value`, called with `Invoke`.
- A Go function passed to JavaScript that blocks, for example by sleeping, waiting on a channel or calling an `await` import, returns a Promise to JavaScript. JavaScript code that calls it without awaiting the result, such as `xs.map(f)`, gets Promises instead of values.
- The generated module's TypeScript types of imported functions are `any`.
- Go declarations are written by hand; they are not generated from `.d.ts` files.
