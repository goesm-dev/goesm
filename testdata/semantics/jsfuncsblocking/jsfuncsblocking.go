// Package jsfuncsblocking is package jsfuncs with a Go function that blocks:
// every function made by js.FuncOf may then return a Promise of its result.
package jsfuncsblocking

import "syscall/js"

// Setup stores the functions in globalThis.jsfuncsblocking.
func Setup() {
	js.Global().Set("jsfuncsblocking", js.ValueOf(map[string]any{
		"value":   js.FuncOf(func(this js.Value, args []js.Value) any { return args[0].Int() * 2 }),
		"resolve": js.FuncOf(func(this js.Value, args []js.Value) any { return promise(false, args[0]) }),
		"reject":  js.FuncOf(func(this js.Value, args []js.Value) any { return promise(true, args[0]) }),
		"blocking": js.FuncOf(func(this js.Value, args []js.Value) any {
			ch := make(chan int)
			go func() { ch <- args[0].Int() * 3 }()
			return <-ch
		}),
		"blockingReject": js.FuncOf(func(this js.Value, args []js.Value) any {
			ch := make(chan js.Value)
			go func() { ch <- promise(true, args[0]) }()
			return <-ch
		}),
		"blockingPanic": js.FuncOf(func(this js.Value, args []js.Value) any {
			ch := make(chan string)
			go func() { ch <- args[0].String() }()
			panic(<-ch)
		}),
	}))
}

// promise returns a JavaScript Promise settled with v.
func promise(reject bool, v js.Value) js.Value {
	return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) any {
		if reject {
			args[1].Invoke(js.Global().Get("Error").New(v))
		} else {
			args[0].Invoke(v)
		}
		return nil
	}))
}
