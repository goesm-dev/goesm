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
