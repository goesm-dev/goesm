//go:build goesm

// goesm's patch of package crypto/internal/fips140/sha3: the gc
// implementation views the state's bytes as words through unsafe.Pointer,
// which goesm has no address space for, so the permutation loads and stores
// the words explicitly.
package sha3

// Rotation offsets and lane order of the ρ and π steps.
var (
	keccakRotc = [24]int{1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14, 27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44}
	keccakPiln = [24]int{10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4, 15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1}
)

// keccakF1600Generic applies the Keccak permutation.
func keccakF1600Generic(da *[200]byte) {
	var a [25]uint64
	for i := range a {
		a[i] = byteorder.LEUint64(da[i*8:])
	}
	var c [5]uint64
	for round := range 24 {
		// θ
		for x := range 5 {
			c[x] = a[x] ^ a[x+5] ^ a[x+10] ^ a[x+15] ^ a[x+20]
		}
		for x := range 5 {
			d := c[(x+4)%5] ^ bits.RotateLeft64(c[(x+1)%5], 1)
			for y := 0; y < 25; y += 5 {
				a[y+x] ^= d
			}
		}
		// ρ and π
		cur := a[1]
		for i, j := range keccakPiln {
			cur, a[j] = a[j], bits.RotateLeft64(cur, keccakRotc[i])
		}
		// χ
		for y := 0; y < 25; y += 5 {
			b0, b1, b2, b3, b4 := a[y], a[y+1], a[y+2], a[y+3], a[y+4]
			a[y] = b0 ^ (^b1 & b2)
			a[y+1] = b1 ^ (^b2 & b3)
			a[y+2] = b2 ^ (^b3 & b4)
			a[y+3] = b3 ^ (^b4 & b0)
			a[y+4] = b4 ^ (^b0 & b1)
		}
		// ι
		a[0] ^= rc[round]
	}
	for i := range a {
		byteorder.LEPutUint64(da[i*8:], a[i])
	}
}
