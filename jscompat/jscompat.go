// Package jscompat gives Go code what JavaScript does differently from Go's
// standard library: string offsets in UTF-16 code units, Math.round's
// rounding, and, in a js build, the engine's Math functions, Unicode
// normalization and regular expressions (RegExp).
//
// The functions of this file are plain Go and build everywhere. With
// goesm, the others call the engine directly through //goesm:import; with
// Go's own js/wasm port, through syscall/js.
package jscompat

import (
	"math"
	"unicode/utf8"
)

// UTF16Len returns the length of s in UTF-16 code units, which is
// JavaScript's length of the same text. An invalid byte counts as one unit,
// as the U+FFFD it becomes.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// UTF16Offset returns the offset in UTF-16 code units, as JavaScript's
// string indices and the selection of an input element count, of the byte
// offset i of s. An offset inside a character is that of the character's
// start.
func UTF16Offset(s string, i int) int {
	i = min(max(i, 0), len(s))
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return UTF16Len(s[:i])
}

// ByteOffset returns the byte offset in s of the offset u in UTF-16 code
// units. An offset between the two halves of a surrogate pair, which Go
// cannot represent, is that of the character's start; one past the end is
// len(s).
func ByteOffset(s string, u int) int {
	n := 0
	for i, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if n+w > u {
			return i
		}
		n += w
	}
	return len(s)
}

// Round returns x rounded to the nearest integer as JavaScript's Math.round
// does: halves round toward positive infinity, so Round(-2.5) is -2, where
// math.Round(-2.5) is -3.
func Round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	if r == 0 && math.Signbit(x) {
		return math.Copysign(0, -1)
	}
	return r
}
