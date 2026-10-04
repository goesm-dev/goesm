//go:build wasm && !tinygo

package main

import (
	"unsafe"

	"example.com/bench/kernels"
)

// See export_wasm.go.

//go:wasmexport add
func add(a, b int32) int32 { return addInt32(a, b) }

//go:wasmexport in
func in(n int32) unsafe.Pointer { return inPtr(n) }

//go:wasmexport out
func out() unsafe.Pointer { return outPtr() }

//go:wasmexport upper
func upper(n int32) int32 { return call(kernels.Upper, n) }

//go:wasmexport handle
func handle(n int32) int32 { return call(kernels.Handle, n) }
