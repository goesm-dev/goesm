//go:build tinygo

package main

import "example.com/bench/kernels"

// add is Add as a plain WebAssembly export, which TinyGo programs usually
// use for numbers instead of a syscall/js callback.
//
//export add
func add(a, b int32) int32 {
	return int32(kernels.Add(int(a), int(b)))
}
