//go:build tinygo

package main

import (
	"unsafe"

	"example.com/bench/kernels"
)

// See export_wasm.go.

//export add
func add(a, b int32) int32 { return addInt32(a, b) }

//export in
func in(n int32) unsafe.Pointer { return inPtr(n) }

//export out
func out() unsafe.Pointer { return outPtr() }

//export upper
func upper(n int32) int32 { return call(kernels.Upper, n) }

//export handle
func handle(n int32) int32 { return call(kernels.Handle, n) }
