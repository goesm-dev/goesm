//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of math/big's arith.go: Words are 32 bits wide. goesm's uint
// is a JS number, exact only below 2^53, so the 64-bit word arithmetic of
// bits.Mul and friends on uint cannot be done on it; math/big supports
// 32-bit platforms, and with _W = 32 its words are uint32, whose arithmetic
// is exact. The functions below are the originals with the 32-bit variants
// of the math/bits functions.
package big

import "math/bits"

type Word uint32

const (
	_S = _W / 8 // word size in bytes

	_W = 32      // word size in bits
	_B = 1 << _W // digit base
	_M = _B - 1  // digit mask
)

func nlz(x Word) uint {
	return uint(bits.LeadingZeros32(uint32(x)))
}

func mulWW(x, y Word) (z1, z0 Word) {
	hi, lo := bits.Mul32(uint32(x), uint32(y))
	return Word(hi), Word(lo)
}

func mulAddWWW_g(x, y, c Word) (z1, z0 Word) {
	hi, lo := bits.Mul32(uint32(x), uint32(y))
	var cc uint32
	lo, cc = bits.Add32(lo, uint32(c), 0)
	return Word(hi + cc), Word(lo)
}

func addVV_g(z, x, y []Word) (c Word) {
	if len(x) != len(z) || len(y) != len(z) {
		panic("addVV len")
	}

	for i := range z {
		zi, cc := bits.Add32(uint32(x[i]), uint32(y[i]), uint32(c))
		z[i] = Word(zi)
		c = Word(cc)
	}
	return
}

func subVV_g(z, x, y []Word) (c Word) {
	if len(x) != len(z) || len(y) != len(z) {
		panic("subVV len")
	}

	for i := range z {
		zi, cc := bits.Sub32(uint32(x[i]), uint32(y[i]), uint32(c))
		z[i] = Word(zi)
		c = Word(cc)
	}
	return
}

func addVW(z, x []Word, y Word) (c Word) {
	if len(x) != len(z) {
		panic("addVW len")
	}

	if len(z) == 0 {
		return y
	}
	zi, cc := bits.Add32(uint32(x[0]), uint32(y), 0)
	z[0] = Word(zi)
	if cc == 0 {
		if &z[0] != &x[0] {
			copy(z[1:], x[1:])
		}
		return 0
	}
	for i := 1; i < len(z); i++ {
		xi := x[i]
		if xi != ^Word(0) {
			z[i] = xi + 1
			if &z[0] != &x[0] {
				copy(z[i+1:], x[i+1:])
			}
			return 0
		}
		z[i] = 0
	}
	return 1
}

func addVW_ref(z, x []Word, y Word) (c Word) {
	c = y
	for i := range z {
		zi, cc := bits.Add32(uint32(x[i]), uint32(c), 0)
		z[i] = Word(zi)
		c = Word(cc)
	}
	return
}

func subVW(z, x []Word, y Word) (c Word) {
	if len(x) != len(z) {
		panic("subVW len")
	}

	if len(z) == 0 {
		return y
	}
	zi, cc := bits.Sub32(uint32(x[0]), uint32(y), 0)
	z[0] = Word(zi)
	if cc == 0 {
		if &z[0] != &x[0] {
			copy(z[1:], x[1:])
		}
		return 0
	}
	for i := 1; i < len(z); i++ {
		xi := x[i]
		if xi != 0 {
			z[i] = xi - 1
			if &z[0] != &x[0] {
				copy(z[i+1:], x[i+1:])
			}
			return 0
		}
		z[i] = ^Word(0)
	}
	return 1
}

func subVW_ref(z, x []Word, y Word) (c Word) {
	c = y
	for i := range z {
		zi, cc := bits.Sub32(uint32(x[i]), uint32(c), 0)
		z[i] = Word(zi)
		c = Word(cc)
	}
	return c
}

func addMulVVWW_g(z, x, y []Word, m, a Word) (c Word) {
	if len(x) != len(z) || len(y) != len(z) {
		panic("addMulVVWW len")
	}

	c = a
	for i := range z {
		z1, z0 := mulAddWWW_g(y[i], m, x[i])
		lo, cc := bits.Add32(uint32(z0), uint32(c), 0)
		c, z[i] = Word(cc), Word(lo)
		c += z1
	}
	return
}

func divWW(x1, x0, y, m Word) (q, r Word) {
	s := nlz(y)
	if s != 0 {
		x1 = x1<<s | x0>>(_W-s)
		x0 <<= s
		y <<= s
	}
	d := uint32(y)
	// We know that
	//   m = ⎣(B^2-1)/d⎦-B
	//   ⎣(B^2-1)/d⎦ = m+B
	//   (B^2-1)/d = m+B+delta1    0 <= delta1 <= (d-1)/d
	//   B^2/d = m+B+delta2        0 <= delta2 <= 1
	// The quotient we're trying to compute is
	//   quotient = ⎣(x1*B+x0)/d⎦
	//            = ⎣(x1*B*(B^2/d)+x0*(B^2/d))/B^2⎦
	//            = ⎣(x1*B*(m+B+delta2)+x0*(m+B+delta2))/B^2⎦
	//            = ⎣(x1*m+x1*B+x0)/B + x0*m/B^2 + delta2*(x1*B+x0)/B^2⎦
	// The latter two terms of this three-term sum are between 0 and 1.
	// So we can compute just the first term, and we will be low by at most 2.
	t1, t0 := bits.Mul32(uint32(m), uint32(x1))
	_, c := bits.Add32(t0, uint32(x0), 0)
	t1, _ = bits.Add32(t1, uint32(x1), c)
	// The quotient is either t1, t1+1, or t1+2.
	// We'll try t1 and adjust if needed.
	qq := t1
	// compute remainder r=x-d*q.
	dq1, dq0 := bits.Mul32(d, qq)
	r0, b := bits.Sub32(uint32(x0), dq0, 0)
	r1, _ := bits.Sub32(uint32(x1), dq1, b)
	// The remainder we just computed is bounded above by B+d:
	// r = x1*B + x0 - d*q.
	//   = x1*B + x0 - d*⎣(x1*m+x1*B+x0)/B⎦
	//   = x1*B + x0 - d*((x1*m+x1*B+x0)/B-alpha)                                   0 <= alpha < 1
	//   = x1*B + x0 - x1*d/B*m                         - x1*d - x0*d/B + d*alpha
	//   = x1*B + x0 - x1*d/B*⎣(B^2-1)/d-B⎦             - x1*d - x0*d/B + d*alpha
	//   = x1*B + x0 - x1*d/B*⎣(B^2-1)/d-B⎦             - x1*d - x0*d/B + d*alpha
	//   = x1*B + x0 - x1*d/B*((B^2-1)/d-B-beta)        - x1*d - x0*d/B + d*alpha   0 <= beta < 1
	//   = x1*B + x0 - x1*B + x1/B + x1*d + x1*d/B*beta - x1*d - x0*d/B + d*alpha
	//   =        x0        + x1/B        + x1*d/B*beta        - x0*d/B + d*alpha
	//   = x0*(1-d/B) + x1*(1+d*beta)/B + d*alpha
	//   <  B*(1-d/B) +  d*B/B          + d          because x0<B (and 1-d/B>0), x1<d, 1+d*beta<=B, alpha<1
	//   =  B - d     +  d              + d
	//   = B+d
	// So r1 can only be 0 or 1. If r1 is 1, then we know q was too small.
	// Add 1 to q and subtract d from r. That guarantees that r is <B, so
	// we no longer need to keep track of r1.
	if r1 != 0 {
		qq++
		r0 -= d
	}
	// If the remainder is still too large, increment q one more time.
	if r0 >= d {
		qq++
		r0 -= d
	}
	return Word(qq), Word(r0 >> s)
}

func reciprocalWord(d1 Word) Word {
	u := uint32(d1 << nlz(d1))
	x1 := ^u
	x0 := uint32(_M)
	rec, _ := bits.Div32(x1, x0, u) // (_B^2-1)/U-_B = (_B*(_M-C)+_M)/U
	return Word(rec)
}
