//go:build wasm

package main

import (
	"unsafe"

	"example.com/bench/kernels"
)

// The calling kernels as plain WebAssembly exports, the fast way into Go
// and TinyGo wasm (js/loaders.mjs uses these instead of syscall/js, which
// costs microseconds per call): numbers are passed directly, strings as
// UTF-8 in linear memory. JS asks for an input buffer of n bytes (in),
// writes the argument there and calls upper or handle with its length; the
// result is in the output buffer (out) with the returned length. Go exports
// them with //go:wasmexport (export_gowasm.go), TinyGo with //export
// (export_tinygo.go): its //go:wasmexport resumes the scheduler on every
// call, which costs about 0.2 ms while main blocks.

var inBuf, outBuf []byte

func addInt32(a, b int32) int32 {
	return int32(kernels.Add(int(a), int(b)))
}

func inPtr(n int32) unsafe.Pointer {
	if cap(inBuf) < int(n) {
		inBuf = make([]byte, n)
	}
	inBuf = inBuf[:n]
	return unsafe.Pointer(unsafe.SliceData(inBuf))
}

func outPtr() unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(outBuf))
}

func call(f func(string) string, n int32) int32 {
	outBuf = append(outBuf[:0], f(string(inBuf[:n]))...)
	return int32(len(outBuf))
}
