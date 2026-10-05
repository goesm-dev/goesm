# Calling Go from JavaScript

[日本語](js-exports.ja.md)

The exported functions of the Go package you build are JavaScript functions that take and return ordinary JavaScript values. goesm converts the arguments and the results at the boundary, according to the Go types of the declaration, with the table [`//goesm:import`](js-imports.md) uses in the other direction. Nothing is converted by hand.

```go
package shop

type Item struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

func Total(items []Item) int { ... }
func Label(it Item) string   { ... }
func Parse(s string) (Item, error) { ... }
```

```ts
import { Label, Parse, Total } from "./goesm-ts/example.com/app/shop.ts";

Total([{ name: "りんご", price: 120 }, { name: "パン", price: 250 }]); // 370
Label({ name: "みかん", price: 80 });                              // a JS string
try {
  Parse("x");
} catch (e) {
  console.log(e.name, e.message); // GoError, and the text of err.Error()
}
```

The package you build is the one named on the command line, such as `./shop` in `goesm emit-ts ./shop` or `goesm build ./shop`, and the one a `go:` import of gosfc names. Import its module. The modules of the packages it imports are part of the tree too, but they export their functions with Go's own representation of values, for the generated modules that import them.

## Conversions

| Go | JavaScript |
| --- | --- |
| `bool`, `int`, `float64` and the other numbers | boolean, number, passed as they are |
| `int64`, `uint64` | bigint; an argument may also be a number |
| `string` | string |
| `[]byte` | `Uint8Array`; an argument may also be an array of numbers |
| other slices and arrays | array |
| `map[string]T` | plain object |
| other maps | `Map` |
| struct | plain object with the exported fields, named as `encoding/json` names them (the `json` tag name, or the field name); an embedded struct contributes its fields. Fields an argument leaves out are zero. |
| pointer to a struct type with methods | the Go object itself, a handle (below) |
| other pointers | the value pointed to, converted; `null` for nil. An argument is a pointer to a converted copy |
| `error` | a final `error` result is thrown (below); elsewhere a `GoError` or `null` |
| `any` | the conversion of the dynamic value; an argument becomes what `encoding/json` decodes into an `any` |
| other interfaces | the Go value, a handle |
| `http.Handler` result | a fetch handler, `(request: Request) => Promise<Response>` |
| function | function, which converts its own arguments and results the same way and throws its final `error` result |
| channel, complex number, `unsafe.Pointer` | the Go value, a handle |
| `js.Value`, `js.Func` | the value itself |
| variadic parameter | separate arguments |
| several results | an array of them, without the final `error` |

Values are copied: an array or object changed on one side does not change the other. A Go value JavaScript received is also accepted where its type is expected: a struct value of the package's classes is copied, a slice or a map is taken as it is.

Numbers are not checked: a fraction passed for an `int` reaches Go as it is, so pass integers. TypeScript types both as `number`.

## Handles

A pointer to a struct type with methods is how Go code usually hands out an object with state, so JavaScript receives the Go object itself and calls its methods:

```go
type Cart struct{ owner string; items []Item }

func NewCart(owner string) *Cart { return &Cart{owner: owner} }
func (c *Cart) Add(items ...Item) { c.items = append(c.items, items...) }
func (c *Cart) Total() int        { return Total(c.items) }
```

```ts
const cart = NewCart("かなで");
cart.Add({ name: "みかん", price: 100 }, { name: "柿", price: 80 });
cart.Total(); // 180
```

The methods of a handle convert their arguments and results like the package's functions. Each method is also a function of the module, `Cart$Add(cart, item)`. A handle of another package's type, such as a `*strings.Builder` an exported function returns, has its exported methods too. The fields of a handle hold Go's own representation (a Go string is not a JS string), so read data through methods and functions, which convert it.

A handle, or an object of one of the package's struct classes, passed where an interface is expected is that interface value if its type has the interface's methods.

## Errors and panics

A function whose last result is an `error` returns its other results, or throws the error as a `GoError`: an `Error` whose `name` is `"GoError"` and whose `message` is the text of `err.Error()`. A `GoError` passed back to Go where an `error` is expected is the Go error again, so `errors.Is` and `errors.As` work on it; any other value becomes an error with the same message.

```go
var ErrEmpty = errors.New("empty input")

func SumCSV(s string) (int, error)
func IsEmpty(err error) bool { return errors.Is(err, ErrEmpty) }
```

```ts
try {
  SumCSV(" ");
} catch (e) {
  IsEmpty(e); // true
}
```

A panic that reaches JavaScript is thrown as an `Error` whose `name` is `"GoPanic"`, with the panic value in its message and a stack trace that points at the `.go` files.

## Functions that block

A Go function that blocks, for example on a channel, a mutex, `time.Sleep` or a JavaScript Promise imported with `await`, returns a Promise of its converted result, and its error result rejects the Promise. A function JavaScript passes to Go is called synchronously, and its result is used as it is returned.

## Servers on the edge

An `http.Handler` result is a fetch handler, for Cloudflare Workers, `Deno.serve`, `Bun.serve` and service workers:

```go
func Handler() http.Handler { ... }
```

```ts
import { Handler } from "./goesm-ts/example.com/app/api.ts";
export default { fetch: Handler() };
```

## TypeScript

The exports are typed with the JavaScript types of the table: `Total(items: Array<{ name?: string; price?: number }> | null): number`, a handle with its class, a blocking function with a Promise. TypeScript reports a call with a wrong argument type.

## Performance

A call with numbers costs what a call to a JavaScript function costs: the export passes them as they are. A string is converted from UTF-16 to UTF-8 on the way in and back on the way out; an ASCII string is passed unchanged after a scan. Slices, maps and structs are copied element by element. The calling kernels of [bench](../bench) measure the cost: `Add`, `Upper` and `Handle` call Go with numbers and strings 100,000 or 10,000 times.
