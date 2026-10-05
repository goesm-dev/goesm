package main

import (
	"fmt"
	"syscall/js"
)

// syscallJS covers the syscall/js calls goesm lowers to direct JavaScript
// calls (internal/lower/jsvalue.go) next to the ones it does not.
func syscallJS() {
	o := js.Global().Get("Object").New()
	o.Set("ascii", 1)
	o.Set("名前", "ペン")
	name := "価格"
	o.Set(name, int64(120))
	o.Set("big", uint64(1<<53))
	o.Set("ok", true)
	o.Set("none", nil)
	o.Set("self", o.Get("ascii"))
	o.Set("list", []any{1, "a"}) // not lowered: a slice goes through ValueOf
	fmt.Println(js.Global().Get("JSON").Call("stringify", o).String())
	fmt.Println(o.Get("名前").String(), o.Get(name).Int(), o.Get("big").Float())

	arr := js.Global().Get("Array").New(3)
	arr.SetIndex(0, "零")
	arr.SetIndex(1, 1.5)
	arr.SetIndex(2, o)
	fmt.Println(arr.Length(), arr.Index(0).String(), arr.Index(1).Float(), arr.Index(2).Get("ascii").Int())

	s := js.Global().Get("String")
	fmt.Println(s.Invoke(42).Type(), s.Get("prototype").Get("concat").Call("call", "日本語", "と", 2, true, nil).String())
	args := []any{"x", "a", "b"}
	fmt.Println(s.Get("prototype").Get("concat").Call("call", args...).String())
	fmt.Println(js.Global().Get("Math").Call("max", 3, o.Get("ascii"), 2.5).Float())
	double := js.FuncOf(func(this js.Value, args []js.Value) any { return args[0].Int() * 2 })
	fmt.Println(js.Global().Get("Array").Call("of", 1, 2).Call("map", double).Call("join", "+").String())

	for _, f := range []func(){
		func() { js.ValueOf(1).Get("x") },
		func() { js.ValueOf("s").Set("x", 1) },
		func() { js.Null().SetIndex(0, 1) },
		func() { o.Call("ないメソッド", 1) },
		func() { js.ValueOf(true).Call("f") },
		func() { js.ValueOf("s").Call("concat", "t") },
		func() { js.ValueOf("s").Call("concat", []any{"t"}...) },
		func() { o.Invoke(1) },
		func() { o.New() },
		func() { js.Global().Get("JSON").Call("parse", "{") },
		func() { js.Global().Get("Symbol").New() },
	} {
		func() {
			defer func() {
				r := recover()
				if err, ok := r.(js.Error); ok {
					r = "js.Error " + err.Get("name").String()
				}
				fmt.Println("recovered:", r)
			}()
			f()
		}()
	}
}
