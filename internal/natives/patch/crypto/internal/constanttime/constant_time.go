//go:build goesm

// goesm's patch of package crypto/internal/constanttime: gc replaces
// boolToUint8 with a branch-free instruction sequence. JavaScript engines
// make no constant-time guarantees either way.
package constanttime

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
