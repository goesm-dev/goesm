//go:build js

// Package promise waits for JavaScript Promises in Go code that uses
// syscall/js, in goesm's output and in Go's own js/wasm port.
package promise

import "syscall/js"

// Await waits until the Promise p settles and returns its value, or its
// rejection reason as a js.Error, so that errors.As reaches the reason (a
// DOMException named "AbortError", say). A value that is not a Promise is
// returned as it is, as JavaScript's await does.
//
// goesm imports the function below from await.ts, as a function that
// blocks until the Promise settles (its body is for Go's js/wasm port).
//
//goesm:import "./await.ts" settle await
func Await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	resolve := js.FuncOf(func(this js.Value, args []js.Value) any {
		ch <- result{v: args[0]}
		return nil
	})
	defer resolve.Release()
	reject := js.FuncOf(func(this js.Value, args []js.Value) any {
		ch <- result{err: js.Error{Value: args[0]}}
		return nil
	})
	defer reject.Release()
	js.Global().Get("Promise").Call("resolve", p).Call("then", resolve, reject)
	r := <-ch
	return r.v, r.err
}
