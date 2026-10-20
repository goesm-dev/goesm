//go:build goesm

// goesm's patch of package syscall's fs_js.go: the host's fs, process and
// path come from the runtime (hostValue in runtime/src/natives.ts) rather
// than from properties of the global object, which wasm_exec.js sets and
// goesm does not. Before, js.Value.Get looked for these three names on
// every property read of the global object, so every program that read
// one carried the file system shims.
package syscall

import "syscall/js"

var jsProcess = host("process")
var jsPath = host("path")
var jsFS = host("fs")

func host(name string) (v js.Value) {
	hostValue(&v, name)
	return
}

// hostValue sets *v to the host's object name: node:fs adapted to the
// callback API (the console in browsers), process (a stand-in in
// browsers) or node:path.
func hostValue(v *js.Value, name string)
