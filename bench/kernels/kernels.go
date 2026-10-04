// Package kernels holds the workloads of goesm's comparison benchmark.
//
// Every kernel takes a size and returns a checksum, so the JS harness calls
// each compiled version the same way and checks it against native Go. The
// kernels keep `int` values below 2^31 because `int` is 32 bits wide under
// GopherJS and TinyGo's wasm target, and wrap explicitly typed integers
// (uint32, uint64) where overflow is part of the algorithm.
package kernels

// Fib is the naive recursive Fibonacci number: function calls and int
// arithmetic.
func Fib(n int) int {
	if n < 2 {
		return n
	}
	return Fib(n-1) + Fib(n-2)
}

// Sieve counts the primes below n with the sieve of Eratosthenes: a []bool
// and tight loops.
func Sieve(n int) int {
	composite := make([]bool, n)
	count := 0
	for i := 2; i < n; i++ {
		if composite[i] {
			continue
		}
		count++
		if i > (n-1)/i {
			continue // i*i is past the end (and would overflow a 32-bit int)
		}
		for j := i * i; j < n; j += i {
			composite[j] = true
		}
	}
	return count
}

// Mandelbrot counts the points of a size×size grid that stay bounded for
// 50 iterations: float64 arithmetic in nested loops.
func Mandelbrot(size int) int {
	inside := 0
	for y := 0; y < size; y++ {
		ci := 2*float64(y)/float64(size) - 1
		for x := 0; x < size; x++ {
			cr := 2*float64(x)/float64(size) - 1.5
			zr, zi := 0.0, 0.0
			i := 0
			for ; i < 50 && zr*zr+zi*zi <= 4; i++ {
				zr, zi = zr*zr-zi*zi+cr, 2*zr*zi+ci
			}
			if i == 50 {
				inside++
			}
		}
	}
	return inside
}

// testBytes returns n deterministic bytes.
func testBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

// FNV32 hashes n bytes eight times over with FNV-1a 32 (uint32 multiply
// and xor). Each round continues from the last one's hash, so a compiler
// cannot drop the first seven.
func FNV32(n int) int {
	data := testBytes(n)
	h := uint32(2166136261)
	for r := 0; r < 8; r++ {
		for _, c := range data {
			h ^= uint32(c)
			h *= 16777619
		}
	}
	return int(h >> 1)
}

// FNV64 is FNV32 with FNV-1a 64: uint64 arithmetic.
func FNV64(n int) int {
	data := testBytes(n)
	h := uint64(14695981039346656037)
	for r := 0; r < 8; r++ {
		for _, c := range data {
			h ^= uint64(c)
			h *= 1099511628211
		}
	}
	return int(h >> 34)
}

// lcg is a 32-bit linear congruential generator for deterministic inputs.
type lcg uint32

func (r *lcg) next() uint32 {
	*r = *r*1664525 + 1013904223
	return uint32(*r)
}
