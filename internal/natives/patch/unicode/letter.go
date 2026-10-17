//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of unicode's letter.go: ToUpper and ToLower of a rune
// beyond ASCII are the engine's toUpperCase and toLowerCase (upperRune and
// lowerRune in runtime/src/natives.ts), so that a program converting case
// does not keep CaseRanges, about 10 KB minified. Where the engine maps a
// rune to several runes (full case mapping), lowerRune and upperRune give
// Go's simple mapping. A case pair added in a Unicode version newer than the
// engine's is left unmapped.
package unicode

// ToUpper maps the rune to upper case.
func ToUpper(r rune) rune {
	if r <= MaxASCII {
		if 'a' <= r && r <= 'z' {
			r -= 'a' - 'A'
		}
		return r
	}
	return upperRune(r)
}

// ToLower maps the rune to lower case.
func ToLower(r rune) rune {
	if r <= MaxASCII {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return r
	}
	return lowerRune(r)
}

func upperRune(r rune) rune
func lowerRune(r rune) rune
