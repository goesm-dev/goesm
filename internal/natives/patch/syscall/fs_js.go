//go:build goesm

// goesm's patch of package syscall's fs_js.go: the host's fs, process and
// path come from the runtime (hostValue in runtime/src/natives.ts) rather
// than from properties of the global object, which wasm_exec.js sets and
// goesm does not. Before, js.Value.Get looked for these three names on
// every property read of the global object, so every program that read
// one carried the file system shims.
package syscall

import "syscall/js"

// The host's objects and the flags of its fs come from natives without
// effects (effFree in internal/lower/effects.go), and the declarations they
// replace go with their initializers, so that a program that never touches
// a file leaves them, the file system shims and syscall/js out.
//
//goesm:droporiginal
var (
	jsProcess  = host("process")
	jsPath     = host("path")
	jsFS       = host("fs")
	uint8Array = host("Uint8Array")

	// constants is unused: the node flags come from hostFlag.
	constants js.Value

	nodeWRONLY    = hostFlag("O_WRONLY")
	nodeRDWR      = hostFlag("O_RDWR")
	nodeCREATE    = hostFlag("O_CREAT")
	nodeTRUNC     = hostFlag("O_TRUNC")
	nodeAPPEND    = hostFlag("O_APPEND")
	nodeEXCL      = hostFlag("O_EXCL")
	nodeDIRECTORY = hostFlag("O_DIRECTORY")
)

// init replaces the one that reads O_DIRECTORY, which hostFlag reads.
func init() {}

// hostFlag returns the flag name of the host's fs.constants, or -1 if it
// has none (Node on Windows has no O_DIRECTORY).
func hostFlag(name string) int

func host(name string) (v js.Value) {
	hostValue(&v, name)
	return
}

// hostValue sets *v to the host's object name: node:fs adapted to the
// callback API (the console in browsers), process (a stand-in in
// browsers), node:path or Uint8Array.
func hostValue(v *js.Value, name string)
