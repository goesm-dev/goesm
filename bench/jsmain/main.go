//go:build js

// Command jsmain exposes the kernels to JavaScript through syscall/js, for
// the compilers that build programs rather than packages: GopherJS, Go's
// js/wasm port and TinyGo's wasm target. It sets globalThis.goBench to an
// object with one function per kernel, then blocks so the functions stay
// callable. Channels returns a Promise: a syscall/js callback must not block,
// so the kernel runs on its own goroutine.
package main

import (
	"syscall/js"

	"example.com/bench/kernels"
)

var byName = map[string]func(int) int{
	"Fib":         kernels.Fib,
	"Sieve":       kernels.Sieve,
	"Mandelbrot":  kernels.Mandelbrot,
	"FNV32":       kernels.FNV32,
	"FNV64":       kernels.FNV64,
	"NBody":       kernels.NBody,
	"BinaryTrees": kernels.BinaryTrees,
	"Interfaces":  kernels.Interfaces,
	"MapInt":      kernels.MapInt,
	"MapString":   kernels.MapString,
	"Strings":     kernels.Strings,
	"Sort":        kernels.Sort,
	"JSON":        kernels.JSON,
	"Sprintf":     kernels.Sprintf,
}

func main() {
	exports := js.Global().Get("Object").New()
	for name, f := range byName {
		f := f
		exports.Set(name, js.FuncOf(func(this js.Value, args []js.Value) any {
			return f(args[0].Int())
		}))
	}
	exports.Set("Channels", js.FuncOf(func(this js.Value, args []js.Value) any {
		n := args[0].Int()
		return js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) any {
			resolve := args[0]
			go func() { resolve.Invoke(kernels.Channels(n)) }()
			return nil
		}))
	}))
	exports.Set("Add", js.FuncOf(func(this js.Value, args []js.Value) any {
		return kernels.Add(args[0].Int(), args[1].Int())
	}))
	js.Global().Set("goBench", exports)
	select {}
}
