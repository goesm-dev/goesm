// Command jsimport measures calls from Go to JavaScript: through
// //goesm:import declarations, through syscall/js, and, as the reference,
// the same calls made from JavaScript itself.
//
//	cd bench && go run ../cmd/goesm build -o /tmp/jsimport ./jsimport && node /tmp/jsimport/jsimport.js
package main

import (
	"fmt"
	"syscall/js"
	"time"
)

type Item struct {
	Price    int `json:"price"`
	Quantity int `json:"quantity"`
}

//goesm:import "./lib.mjs" add
func add(a, b int) int

//goesm:import "./lib.mjs" strlen
func strlen(s string) int

//goesm:import "./lib.mjs" upper
func upper(s string) string

//goesm:import "./lib.mjs" total
func total(item Item) int

//goesm:import "./lib.mjs" sum
func sum(xs []float64) float64

//goesm:import "./lib.mjs" jsLoop
func jsLoop(n int, f js.Value, args ...any) float64

//goesm:import "./lib.mjs" *
var lib js.Value

const n = 1_000_000

// Typed sinks, so that measuring does not box results in interfaces.
var (
	sinkInt   int
	sinkFloat float64
	sinkStr   string
)

func measure(name string, f func()) float64 {
	for i := 0; i < n/10; i++ { // warm up
		f()
	}
	t0 := time.Now()
	for i := 0; i < n; i++ {
		f()
	}
	return float64(time.Since(t0).Nanoseconds()) / n
}

func main() {
	ascii, ja := "hello, world", "こんにちは、世界"
	item := Item{Price: 120, Quantity: 3}
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	jsXs := js.ValueOf([]any{1, 2, 3, 4, 5, 6, 7, 8})
	jsItem := js.ValueOf(map[string]any{"price": 120, "quantity": 3})
	rows := []struct {
		name              string
		imported, syscall func()
		jsArgs            []any
		jsF               string
	}{
		{"add(int, int) int", func() { sinkInt = add(1, 2) }, func() { sinkInt = lib.Call("add", 1, 2).Int() }, []any{1, 2}, "add"},
		{"strlen(ASCII string) int", func() { sinkInt = strlen(ascii) }, func() { sinkInt = lib.Call("strlen", ascii).Int() }, []any{ascii}, "strlen"},
		{"strlen(Japanese string) int", func() { sinkInt = strlen(ja) }, func() { sinkInt = lib.Call("strlen", ja).Int() }, []any{ja}, "strlen"},
		{"upper(ASCII string) string", func() { sinkStr = upper(ascii) }, func() { sinkStr = lib.Call("upper", ascii).String() }, []any{ascii}, "upper"},
		{"total(struct) int", func() { sinkInt = total(item) }, func() {
			o := js.Global().Get("Object").New()
			o.Set("price", item.Price)
			o.Set("quantity", item.Quantity)
			sinkInt = lib.Call("total", o).Int()
		}, []any{jsItem}, "total"},
		{"sum([]float64 of 8) float64", func() { sinkFloat = sum(xs) }, func() {
			a := js.Global().Get("Array").New(len(xs))
			for i, x := range xs {
				a.SetIndex(i, x)
			}
			sinkFloat = lib.Call("sum", a).Float()
		}, []any{jsXs}, "sum"},
	}
	fmt.Println("| Call | JS → JS | Go → JS, //goesm:import | Go → JS, syscall/js |")
	fmt.Println("| --- | ---: | ---: | ---: |")
	for _, r := range rows {
		ref := jsLoop(n, lib.Get(r.jsF), r.jsArgs...)
		fmt.Printf("| `%s` | %.1f ns | %.1f ns | %.1f ns |\n", r.name, ref, measure(r.name, r.imported), measure(r.name, r.syscall))
	}
}
