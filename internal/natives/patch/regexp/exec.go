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

// find finds the leftmost match in b, or s if b is nil, appends the
// position of its subexpressions to dstCap and returns dstCap. r is always
// nil: the methods that read a RuneReader call findReader.
//
// nil is returned if no matches are found and non-nil if matches are found.
//
// Go's engines read their input through the input interface, whose step
// method blocks for a RuneReader that waits on the host. goesm decides
// which functions are async by the methods an interface call may reach,
// so with Go's find, one RuneReader in the program that blocks, read by
// one pattern the lowering does not translate, made every method of every
// Regexp async, those of the patterns matched with a RegExp too. find
// never calls the engines with a RuneReader, and calls them with an input
// of a concrete type (findIn), so goesm calls their synchronous clones.
//
//goesm:original findGo
func (re *Regexp) find(r io.RuneReader, b []byte, s string, pos int, ncap int, dstCap []int) []int {
	if re.js == "" {
		return re.findText(b, s, pos, ncap, dstCap)
	}
	if re.longest {
		return re.goEngine().findText(b, s, pos, ncap, dstCap)
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

// findReader is find for a RuneReader, with Go's engines, which read it
// only as far as the match needs.
func (re *Regexp) findReader(r io.RuneReader, pos int, ncap int, dstCap []int) []int {
	if re.js != "" {
		re = re.goEngine()
	}
	return re.findGo(r, nil, "", pos, ncap, dstCap)
}

// findText is Go's find for b, or s if b is nil.
func (re *Regexp) findText(b []byte, s string, pos int, ncap int, dstCap []int) []int {
	if b != nil {
		return re.findIn(&inputBytes{str: b}, len(b), pos, ncap, dstCap)
	}
	return re.findIn(&inputString{str: s}, len(s), pos, ncap, dstCap)
}

// findIn is Go's find for the input i of length end, which is not a
// RuneReader.
func (re *Regexp) findIn(i input, end int, pos int, ncap int, dstCap []int) []int {
	if dstCap == nil {
		// Make sure 'return dstCap' is non-nil.
		dstCap = arrayNoInts[:0:0]
	}

	if end < re.minInputLen {
		return nil
	}

	if re.onepass != nil {
		return re.onePassIn(i, pos, ncap, dstCap)
	}
	if end < re.maxBitStateLen {
		return re.backtrackIn(i, end, pos, ncap, dstCap)
	}

	m := re.get()
	m.init(ncap)
	if !m.match(i, pos) {
		re.put(m)
		return nil
	}

	dstCap = append(dstCap, m.matchcap...)
	re.put(m)
	return dstCap
}

// onePassIn is doOnePass for the input i.
func (re *Regexp) onePassIn(i input, pos, ncap int, dstCap []int) []int {
	startCond := re.cond
	if startCond == ^syntax.EmptyOp(0) { // impossible
		return nil
	}

	m := newOnePassMachine()
	if cap(m.matchcap) < ncap {
		m.matchcap = make([]int, ncap)
	} else {
		m.matchcap = m.matchcap[:ncap]
	}

	matched := false
	for i := range m.matchcap {
		m.matchcap[i] = -1
	}

	r, r1 := endOfText, endOfText
	width, width1 := 0, 0
	r, width = i.step(pos)
	if r != endOfText {
		r1, width1 = i.step(pos + width)
	}
	var flag lazyFlag
	if pos == 0 {
		flag = newLazyFlag(-1, r)
	} else {
		flag = i.context(pos)
	}
	pc := re.onepass.Start
	inst := &re.onepass.Inst[pc]
	// If there is a simple literal prefix, skip over it.
	if pos == 0 && flag.match(syntax.EmptyOp(inst.Arg)) &&
		len(re.prefix) > 0 && i.canCheckPrefix() {
		// Match requires literal prefix; fast search for it.
		if !i.hasPrefix(re) {
			goto Return
		}
		pos += len(re.prefix)
		r, width = i.step(pos)
		r1, width1 = i.step(pos + width)
		flag = i.context(pos)
		pc = int(re.prefixEnd)
	}
	for {
		inst = &re.onepass.Inst[pc]
		pc = int(inst.Out)
		switch inst.Op {
		default:
			panic("bad inst")
		case syntax.InstMatch:
			matched = true
			if len(m.matchcap) > 0 {
				m.matchcap[0] = 0
				m.matchcap[1] = pos
			}
			goto Return
		case syntax.InstRune:
			if !inst.MatchRune(r) {
				goto Return
			}
		case syntax.InstRune1:
			if r != inst.Rune[0] {
				goto Return
			}
		case syntax.InstRuneAny:
			// Nothing
		case syntax.InstRuneAnyNotNL:
			if r == '\n' {
				goto Return
			}
		// peek at the input rune to see which branch of the Alt to take
		case syntax.InstAlt, syntax.InstAltMatch:
			pc = int(onePassNext(inst, r))
			continue
		case syntax.InstFail:
			goto Return
		case syntax.InstNop:
			continue
		case syntax.InstEmptyWidth:
			if !flag.match(syntax.EmptyOp(inst.Arg)) {
				goto Return
			}
			continue
		case syntax.InstCapture:
			if int(inst.Arg) < len(m.matchcap) {
				m.matchcap[inst.Arg] = pos
			}
			continue
		}
		if width == 0 {
			break
		}
		flag = newLazyFlag(r, r1)
		pos += width
		r, width = r1, width1
		if r != endOfText {
			r1, width1 = i.step(pos + width)
		}
	}

Return:
	if !matched {
		freeOnePassMachine(m)
		return nil
	}

	dstCap = append(dstCap, m.matchcap...)
	freeOnePassMachine(m)
	return dstCap
}

// backtrackIn is backtrack for the input i of length end.
func (re *Regexp) backtrackIn(i input, end int, pos int, ncap int, dstCap []int) []int {
	startCond := re.cond
	if startCond == ^syntax.EmptyOp(0) { // impossible
		return nil
	}
	if startCond&syntax.EmptyBeginText != 0 && pos != 0 {
		// Anchored match, past beginning of text.
		return nil
	}

	b := newBitState()
	b.reset(re.prog, end, ncap)

	// Anchored search must start at the beginning of the input
	if startCond&syntax.EmptyBeginText != 0 {
		if len(b.cap) > 0 {
			b.cap[0] = pos
		}
		if !re.tryBacktrack(b, i, uint32(re.prog.Start), pos) {
			freeBitState(b)
			return nil
		}
	} else {

		// Unanchored search, starting from each possible text position.
		// Notice that we have to try the empty string at the end of
		// the text, so the loop condition is pos <= end, not pos < end.
		// This looks like it's quadratic in the size of the text,
		// but we are not clearing visited between calls to TrySearch,
		// so no work is duplicated and it ends up still being linear.
		width := -1
		for ; pos <= end && width != 0; pos += width {
			if len(re.prefix) > 0 {
				// Match requires literal prefix; fast search for it.
				advance := i.index(re, pos)
				if advance < 0 {
					freeBitState(b)
					return nil
				}
				pos += advance
			}

			if len(b.cap) > 0 {
				b.cap[0] = pos
			}
			if re.tryBacktrack(b, i, uint32(re.prog.Start), pos) {
				// Match must be leftmost; done.
				goto Match
			}
			_, width = i.step(pos)
		}
		freeBitState(b)
		return nil
	}

Match:
	dstCap = append(dstCap, b.matchcap...)
	freeBitState(b)
	return dstCap
}

// execJS matches s from the byte offset pos with the RegExp of source src,
// and returns nil or the byte offsets of the first ncap/2 groups' bounds
// (-1 for a group that did not match).
func execJS(src, s string, pos, ncap int) []int
