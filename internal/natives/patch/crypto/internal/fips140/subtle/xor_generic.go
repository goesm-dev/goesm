//go:build goesm

package subtle

// xorBytes and words are reached only from XORBytes, which goesm replaces
// (see xor.go).
func xorBytes(dstb, xb, yb *byte, n int) { panic("unreachable") }
func words(x []byte) []uintptr           { panic("unreachable") }
