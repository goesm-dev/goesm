//go:build goesm

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of regexp's exec.go: a Regexp compiled to a RegExp
// (regexp.go) finds matches with it (execJS in runtime/src/natives.ts),
// except in a RuneReader or leftmost-longest. A lazyFlag is an int, a
// Number, where Go's is a uint64, a BigInt: the engines make one at each
// position of the input.
package regexp

import (
	"io"
	"regexp/syntax"
)

// A lazyFlag is a lazily-evaluated syntax.EmptyOp,
// for checking zero-width flags like ^ $ \A \z \B \b.
// It records the pair of relevant runes and does not
// determine the implied flags until absolutely necessary
// (most of the time, that means never).
//
// It holds lazyRune of each rune, r1*lazyBase + r2, exact in a float64.
type lazyFlag int

// lazyBase is one more than the largest lazyRune.
const lazyBase = 0x110002

// lazyRune maps r to [0, lazyBase): the runes without one before or after
// them (negative) to 0, those past unicode.MaxRune to lazyBase-1. match
// only asks whether a rune is negative, a newline or a word character,
// which those answer as r does.
func lazyRune(r rune) int {
	if r < 0 {
		return 0
	}
	if r > 0x10FFFF {
		return lazyBase - 1
	}
	return int(r) + 1
}

func newLazyFlag(r1, r2 rune) lazyFlag {
	return lazyFlag(lazyRune(r1)*lazyBase + lazyRune(r2))
}

func (f lazyFlag) match(op syntax.EmptyOp) bool {
	if op == 0 {
		return true
	}
	r1 := rune(int(f)/lazyBase) - 1
	if op&syntax.EmptyBeginLine != 0 {
		if r1 != '\n' && r1 >= 0 {
			return false
		}
		op &^= syntax.EmptyBeginLine
	}
	if op&syntax.EmptyBeginText != 0 {
		if r1 >= 0 {
			return false
		}
		op &^= syntax.EmptyBeginText
	}
	if op == 0 {
		return true
	}
	r2 := rune(int(f)%lazyBase) - 1
	if op&syntax.EmptyEndLine != 0 {
		if r2 != '\n' && r2 >= 0 {
			return false
		}
		op &^= syntax.EmptyEndLine
	}
	if op&syntax.EmptyEndText != 0 {
		if r2 >= 0 {
			return false
		}
		op &^= syntax.EmptyEndText
	}
	if op == 0 {
		return true
	}
	if syntax.IsWordChar(r1) != syntax.IsWordChar(r2) {
		op &^= syntax.EmptyWordBoundary
	} else {
		op &^= syntax.EmptyNoWordBoundary
	}
	return op == 0
}

// find finds the leftmost match in the input, appends the position
// of its subexpressions to dstCap and returns dstCap.
//
// nil is returned if no matches are found and non-nil if matches are found.
//
//goesm:original findGo
func (re *Regexp) find(r io.RuneReader, b []byte, s string, pos int, ncap int, dstCap []int) []int {
	if re.js == "" {
		return re.findGo(r, b, s, pos, ncap, dstCap)
	}
	if r != nil || re.longest {
		return re.goEngine().findGo(r, b, s, pos, ncap, dstCap)
	}
	if b != nil {
		s = string(b)
	}
	m := execJS(re.js, s, pos, ncap)
	if m == nil {
		return nil
	}
	if dstCap == nil {
		// Make sure 'return dstCap' is non-nil.
		dstCap = arrayNoInts[:0:0]
	}
	return append(dstCap, m...)
}

// execJS matches s from the byte offset pos with the RegExp of source src,
// and returns nil or the byte offsets of the first ncap/2 groups' bounds
// (-1 for a group that did not match).
func execJS(src, s string, pos, ncap int) []int
