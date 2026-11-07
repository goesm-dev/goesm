//go:build goesm

package godebug

// writeStderr writes b to standard error.
func writeStderr(b []byte)

// Write writes b to standard error. The gc implementation passes a raw
// pointer to runtime.write, which goesm has no address space for.
func (*runtimeStderr) Write(b []byte) (int, error) {
	writeStderr(b)
	return len(b), nil
}

// write is reached only from runtimeStderr.Write, replaced above.
func write(fd uintptr, p unsafe.Pointer, n int32) int32 { panic("unreachable") }

// updated reports whether lookup handed the runtime update, which it does
// before the first lookup: goesm runs godebug's init only if reachable code
// uses a variable it assigns, which it does not (effFree in
// internal/lower/effects.go), so that a program whose settings are never
// read leaves godebug's cache, its sync.Map and the table of settings out.
// update looks settings up itself, so lookup sets updated first.
var updated bool

// lookup returns the setting of name, the GODEBUG environment applied
// first.
//
//goesm:original lookupGo
func lookup(name string) *setting {
	if !updated {
		updated = true
		setUpdate(update)
	}
	return lookupGo(name)
}
