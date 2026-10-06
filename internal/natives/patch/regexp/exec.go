//go:build goesm

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of regexp's exec.go: a Regexp compiled to a RegExp
// (regexp.go) finds matches with it (execJS in runtime/src/natives.ts).
package regexp

import "io"

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
