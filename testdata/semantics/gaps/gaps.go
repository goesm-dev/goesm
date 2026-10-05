// Package gaps pins down known semantic differences between native Go and
// the PoC. TestKnownGaps asserts these still differ, so documentation and
// reality cannot silently drift apart. Fixing a gap means moving its case to
// a golden fixture.
package gaps

import (
	"math"
	"sync/atomic"
	"time"
)

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

// Goroutines run on one JS thread and switch only where one waits: a
// goroutine that computes without waiting keeps the others from running.
// Native Go runs the new goroutine in parallel, or preempts the busy one.
func Preemption() bool {
	var ran atomic.Bool
	go ran.Store(true)
	for start := time.Now(); time.Since(start) < 50*time.Millisecond; {
	}
	return ran.Load()
}
