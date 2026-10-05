# Using the DOM from Go

[日本語](dom.ja.md)

Go code compiled by goesm uses the DOM, and every other browser API, through `syscall/js`, the package of Go's own WebAssembly port. Code and libraries written for `GOOS=js` build with goesm unchanged, and goesm compiles the common `syscall/js` calls to direct JavaScript property accesses and calls. goesm has no DOM package of its own.

```go
package main

import (
	"strconv"
	"syscall/js"
)

func main() {
	doc := js.Global().Get("document")
	button := doc.Call("createElement", "button")
	button.Set("textContent", "0")
	count := 0
	button.Call("addEventListener", "click", js.FuncOf(func(this js.Value, args []js.Value) any {
		count++
		button.Set("textContent", strconv.Itoa(count))
		return nil
	}))
	doc.Get("body").Call("append", button)
}
```

`goesm build -minify ./counter` writes `dist/counter.js`, which a page loads with a module script. The module is 9 KiB with gzip; the same program built with Go's WebAssembly port is 567 KiB with gzip.

```html
<script type="module" src="dist/counter.js"></script>
```

## Typed DOM libraries

[honnef.co/go/js/dom/v2](https://pkg.go.dev/honnef.co/go/js/dom/v2) wraps the DOM in Go types: `Document`, `Element` and its HTML element types, events and more. It works with goesm as it is:

```go
import "honnef.co/go/js/dom/v2"

func main() {
	doc := dom.GetWindow().Document()
	button := doc.CreateElement("button")
	button.SetTextContent("0")
	count := 0
	button.AddEventListener("click", false, func(dom.Event) {
		count++
		button.SetTextContent(strconv.Itoa(count))
	})
	doc.QuerySelector("body").AppendChild(button)
}
```

Its many types and methods make the module larger: the counter above is 50 KiB with gzip, against 704 KiB for Go's WebAssembly port.

## Event handlers

`js.FuncOf` makes a JavaScript function that calls a Go function. The Go function runs synchronously until it first waits, for example on a channel, `time.Sleep`, an HTTP request or an `await` import ([js-imports.md](js-imports.md)). A handler that waits returns a Promise to JavaScript, which the DOM ignores, so a handler calls `preventDefault` or `stopPropagation` before its first wait:

```go
form.Call("addEventListener", "submit", js.FuncOf(func(this js.Value, args []js.Value) any {
	args[0].Call("preventDefault") // before the request below
	resp, err := http.Post("/api/orders", "application/json", body)
	// ...
	return nil
}))
```

The garbage collector reclaims a function made by `js.FuncOf` together with the JavaScript function, so `Release` is not needed; calling it does nothing. Goroutines and timers run on the page's event loop ([concurrency.md](concurrency.md)).

## Performance

`Value.Get`, `Set`, `SetIndex`, `Call`, `Invoke` and `New` compile to direct JavaScript property accesses and calls when every argument is a boolean, a number, a string, a `js.Value`, a `js.Func` or `nil`. A constant ASCII property or method name, the usual case, is used as it is. Other arguments, such as a slice or a map, are converted by `js.ValueOf` at run time, as in Go.

A call through `syscall/js` costs about 20 to 50 ns more than the same call from JavaScript on Node.js ([js-imports.md](js-imports.md#performance)), which is negligible for event handlers and for building a page. Code that crosses the boundary for each element of a large data set, such as drawing a chart point by point, is faster written in TypeScript and called through `//goesm:import`, which passes a whole slice or struct in one call:

```go
//goesm:import "./chart.ts" drawPoints
func drawPoints(canvas js.Value, points []Point)
```

## With a UI framework

For an application with many components, a framework keeps the DOM in sync with the state and Go supplies the logic. [gosfc](https://github.com/goesm-dev/gosfc) puts Go in Vue components (`<script setup lang="go">`) and in Astro pages, and React or any other framework imports the goesm output as an ES module ([use-cases.md](use-cases.md)).
