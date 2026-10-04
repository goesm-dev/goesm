//go:build goesm

// goesm's patch of package crypto/internal/fips140/subtle: xorBytes works on
// raw pointers (unsafe.Slice), which goesm has no address space for, so
// XORBytes xors the slices directly.
package subtle

// XORBytes sets dst[i] = x[i] ^ y[i] for all i < n = min(len(x), len(y)),
// returning n, the number of bytes written to dst.
// If dst does not have length at least n,
// XORBytes panics without writing anything to dst.
//
// dst and x or y may overlap exactly or not at all,
// otherwise XORBytes may panic.
func XORBytes(dst, x, y []byte) int {
	n := min(len(x), len(y))
	if n == 0 {
		return 0
	}
	if n > len(dst) {
		panic("subtle.XORBytes: dst too short")
	}
	if alias.InexactOverlap(dst[:n], x[:n]) || alias.InexactOverlap(dst[:n], y[:n]) {
		panic("subtle.XORBytes: invalid overlap")
	}
	dst, x, y = dst[:n], x[:n], y[:n]
	for i := range dst {
		dst[i] = x[i] ^ y[i]
	}
	return n
}
