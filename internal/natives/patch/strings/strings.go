//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strings' strings.go: ToUpper and ToLower of an ASCII
// string are the engine's toUpperCase and toLowerCase (isASCII,
// upperASCII and lowerASCII in runtime/src/natives.ts), which map exactly
// a-z and A-Z on ASCII, instead of a loop through a Builder. Split with a
// separator and Join are the engine's split and join: a Go string's code
// units are its bytes, so they cut and concatenate the same bytes.
package strings

import "unicode"

// ToUpper returns s with all Unicode letters mapped to their upper case.
func ToUpper(s string) string {
	if isASCII(s) {
		return upperASCII(s)
	}
	return Map(unicode.ToUpper, s)
}

// ToLower returns s with all Unicode letters mapped to their lower case.
func ToLower(s string) string {
	if isASCII(s) {
		return lowerASCII(s)
	}
	return Map(unicode.ToLower, s)
}

// Split slices s into all substrings separated by sep and returns a slice of
// the substrings between those separators.
func Split(s, sep string) []string {
	if sep == "" {
		return genSplit(s, sep, 0, -1)
	}
	return splitAll(s, sep)
}

// Join concatenates the elements of its first argument to create a single string. The separator
// string sep is placed between elements in the resulting string.
func Join(elems []string, sep string) string { return joinAll(elems, sep) }

func splitAll(s, sep string) []string
func joinAll(elems []string, sep string) string
func isASCII(s string) bool
func upperASCII(s string) string
func lowerASCII(s string) string
