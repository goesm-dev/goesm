// Package jsfuncs exposes Go functions to JavaScript through js.FuncOf. None
// of them blocks, so JavaScript gets their results directly.
package jsfuncs

import "syscall/js"

// Setup stores the functions in globalThis.jsfuncs.
func Setup() {
	js.Global().Set("jsfuncs", js.ValueOf(map[string]any{
		"value":   js.FuncOf(func(this js.Value, args []js.Value) any { return args[0].Int() * 2 }),
		"resolve": js.FuncOf(func(this js.Value, args []js.Value) any { return promise(false, args[0]) }),
		"reject":  js.FuncOf(func(this js.Value, args []js.Value) any { return promise(true, args[0]) }),
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
