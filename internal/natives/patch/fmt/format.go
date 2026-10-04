//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of fmt's format.go: fmtInteger computes the digits of
// values below 2^53 on a uint (a JS number) instead of the uint64 (a
// BigInt), which is several times faster.
package fmt

// fmtInteger formats signed and unsigned integers.
func (f *fmt) fmtInteger(u uint64, base int, isSigned bool, verb rune, digits string) {
	negative := isSigned && int64(u) < 0
	if negative {
		u = -u
	}

	buf := f.intbuf[0:]
	// The already allocated f.intbuf with a capacity of 68 bytes
	// is large enough for integer formatting when no precision or width is set.
	if f.widPresent || f.precPresent {
		// Account 3 extra bytes for possible addition of a sign and "0x".
		width := 3 + f.wid + f.prec // wid and prec are always positive.
		if width > len(buf) {
			// We're going to need a bigger boat.
			buf = make([]byte, width)
		}
	}

	// Two ways to ask for extra leading zero digits: %.3d or %03d.
	// If both are specified the f.zero flag is ignored and
	// padding with spaces is used instead.
	prec := 0
	if f.precPresent {
		prec = f.prec
		// Precision of 0 and value of 0 means "print nothing" but padding.
		if prec == 0 && u == 0 {
			oldZero := f.zero
			f.zero = false
			f.writePadding(f.wid)
			f.zero = oldZero
			return
		}
	} else if f.zero && !f.minus && f.widPresent { // Zero padding is allowed only to the left.
		prec = f.wid
		if negative || f.plus || f.space {
			prec-- // leave room for sign
		}
	}

	// Because printing is easier right-to-left: format u into buf, ending at buf[i].
	// We could make things marginally faster by splitting the 32-bit case out
	// into a separate block but it's not worth the duplication, so u has 64 bits.
	i := len(buf)
	// goesm: u is a BigInt; below 2^53 its digits are computed on a uint,
	// a JS number, and the loops below find u < base.
	if u < 1<<53 {
		v := uint(u)
		switch base {
		case 10:
			for v >= 10 {
				i--
				next := v / 10
				buf[i] = byte('0' + (v - next*10)) // '0'+v can pass 2^53
				v = next
			}
		case 16:
			for v >= 16 {
				i--
				buf[i] = digits[v&0xF]
				v >>= 4
			}
		case 8:
			for v >= 8 {
				i--
				buf[i] = byte('0' + v&7)
				v >>= 3
			}
		case 2:
			for v >= 2 {
				i--
				buf[i] = byte('0' + v&1)
				v >>= 1
			}
		}
		u = uint64(v)
	}
	// Use constants for the division and modulo for more efficient code.
	// Switch cases ordered by popularity.
	switch base {
	case 10:
		for u >= 10 {
			i--
			next := u / 10
			buf[i] = byte('0' + u - next*10)
			u = next
		}
	case 16:
		for u >= 16 {
			i--
			buf[i] = digits[u&0xF]
			u >>= 4
		}
	case 8:
		for u >= 8 {
			i--
			buf[i] = byte('0' + u&7)
			u >>= 3
		}
	case 2:
		for u >= 2 {
			i--
			buf[i] = byte('0' + u&1)
			u >>= 1
		}
	default:
		panic("fmt: unknown base; can't happen")
	}
	i--
	buf[i] = digits[u]
	for i > 0 && prec > len(buf)-i {
		i--
		buf[i] = '0'
	}

	// Various prefixes: 0x, -, etc.
	if f.sharp {
		switch base {
		case 2:
			// Add a leading 0b.
			i--
			buf[i] = 'b'
			i--
			buf[i] = '0'
		case 8:
			if buf[i] != '0' {
				i--
				buf[i] = '0'
			}
		case 16:
			// Add a leading 0x or 0X.
			i--
			buf[i] = digits[16]
			i--
			buf[i] = '0'
		}
	}
	if verb == 'O' {
		i--
		buf[i] = 'o'
		i--
		buf[i] = '0'
	}

	if negative {
		i--
		buf[i] = '-'
	} else if f.plus {
		i--
		buf[i] = '+'
	} else if f.space {
		i--
		buf[i] = ' '
	}

	// Left padding with zeros has already been handled like precision earlier
	// or the f.zero flag is ignored due to an explicitly set precision.
	oldZero := f.zero
	f.zero = false
	f.pad(buf[i:])
	f.zero = oldZero
}
