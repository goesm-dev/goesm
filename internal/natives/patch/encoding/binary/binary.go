//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/binary's binary.go: the 64-bit
// functions work on the two 32-bit halves. A uint64 is a BigInt under
// goesm and a uint32 a JS number, so each half is put together or taken
// apart with number arithmetic, and only joining or splitting the halves
// computes on BigInts: 3 BigInt operations instead of 15 for a load, 2
// instead of 8 for a store.
package binary

func (littleEndian) Uint64(b []byte) uint64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	lo := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	hi := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	return uint64(lo) | uint64(hi)<<32
}

func (littleEndian) PutUint64(b []byte, v uint64) {
	_ = b[7] // early bounds check to guarantee safety of writes below
	lo, hi := uint32(v), uint32(v>>32)
	b[0] = byte(lo)
	b[1] = byte(lo >> 8)
	b[2] = byte(lo >> 16)
	b[3] = byte(lo >> 24)
	b[4] = byte(hi)
	b[5] = byte(hi >> 8)
	b[6] = byte(hi >> 16)
	b[7] = byte(hi >> 24)
}

func (littleEndian) AppendUint64(b []byte, v uint64) []byte {
	lo, hi := uint32(v), uint32(v>>32)
	return append(b,
		byte(lo),
		byte(lo>>8),
		byte(lo>>16),
		byte(lo>>24),
		byte(hi),
		byte(hi>>8),
		byte(hi>>16),
		byte(hi>>24),
	)
}

func (bigEndian) Uint64(b []byte) uint64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	hi := uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	lo := uint32(b[7]) | uint32(b[6])<<8 | uint32(b[5])<<16 | uint32(b[4])<<24
	return uint64(lo) | uint64(hi)<<32
}

func (bigEndian) PutUint64(b []byte, v uint64) {
	_ = b[7] // early bounds check to guarantee safety of writes below
	lo, hi := uint32(v), uint32(v>>32)
	b[0] = byte(hi >> 24)
	b[1] = byte(hi >> 16)
	b[2] = byte(hi >> 8)
	b[3] = byte(hi)
	b[4] = byte(lo >> 24)
	b[5] = byte(lo >> 16)
	b[6] = byte(lo >> 8)
	b[7] = byte(lo)
}

func (bigEndian) AppendUint64(b []byte, v uint64) []byte {
	lo, hi := uint32(v), uint32(v>>32)
	return append(b,
		byte(hi>>24),
		byte(hi>>16),
		byte(hi>>8),
		byte(hi),
		byte(lo>>24),
		byte(lo>>16),
		byte(lo>>8),
		byte(lo),
	)
}
