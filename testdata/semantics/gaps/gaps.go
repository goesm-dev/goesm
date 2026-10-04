// Package gaps pins down known semantic differences between native Go and
// the PoC. TestKnownGaps asserts these still differ, so documentation and
// reality cannot silently drift apart. Fixing a gap means moving its case to
// a golden fixture.
package gaps

import "math"

// int and uint are JS numbers (int64 and uint64 are BigInt): exact below
// 2^53, without 64-bit wrap-around.
func IntWrap() int {
	x := math.MaxInt32
	x *= x
	return x * x
}

func UintWrap() uint {
	var x uint
	x--
	return x
}

// Go's append growth uses allocator size classes; the PoC approximates it.
func AppendCap() int {
	var s []byte
	s = append(s, 1)
	return cap(s) // gc rounds up to the 8-byte size class
}
