// Package gaps pins down known semantic differences between native Go and
// the PoC. TestKnownGaps asserts these still differ, so documentation and
// reality cannot silently drift apart. Fixing a gap means moving its case to
// a golden fixture.
package gaps

// int/int64/uint64 are JS numbers in the PoC: no 64-bit wrap-around.
func Uint64Wrap() uint64 {
	var x uint64
	x--
	return x
}

// Integers above 2^53 lose precision.
func Int64Precision() int64 {
	x := int64(1) << 62
	return x + 1
}

// Go's append growth uses allocator size classes; the PoC approximates it.
func AppendCap() int {
	var s []byte
	s = append(s, 1)
	return cap(s) // gc rounds up to the 8-byte size class
}
