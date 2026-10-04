//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strings' strings.go: ToUpper and ToLower of an ASCII
// string are the engine's toUpperCase and toLowerCase (isASCII,
// upperASCII and lowerASCII in runtime/src/natives.ts), which map exactly
// a-z and A-Z on ASCII, instead of a loop through a Builder.
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

func isASCII(s string) bool
func upperASCII(s string) string
func lowerASCII(s string) string
