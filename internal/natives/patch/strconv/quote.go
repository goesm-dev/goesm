//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strconv's quote.go: Quote of a string of printable
// ASCII without a quote or a backslash, which it returns unchanged between
// quotes, concatenates instead of appending to a []byte. IsPrint and
// isInGraphicList ask the engine's Unicode tables (isPrint and isSpace in
// runtime/src/natives.ts) instead of isprint.go's, 7 KB of every bundle
// that quotes a string, as the Error method of strconv.NumError does.
// Characters newer than the engine's Unicode version (Node.js 22 has
// Unicode 16, Go 1.27 Unicode 17) are not printable.
package strconv

// Quote returns a double-quoted Go string literal representing s. The
// returned string uses Go escape sequences (\t, \n, \xFF, Ā) for
// control characters and non-printable characters as defined by
// [IsPrint].
func Quote(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < ' ' || c > '~' || c == '"' || c == '\\' {
			return quoteWith(s, '"', false, false)
		}
	}
	return `"` + s + `"`
}

// IsPrint reports whether the rune is defined as printable by Go, with
// the same definition as [unicode.IsPrint]: letters, numbers, punctuation,
// symbols and ASCII space.
func IsPrint(r rune) bool {
	// Fast check for Latin-1
	if r <= 0xFF {
		if 0x20 <= r && r <= 0x7E {
			// All the ASCII is printable from space through DEL-1.
			return true
		}
		if 0xA1 <= r && r <= 0xFF {
			// Similarly for ¡ through ÿ...
			return r != 0xAD // ...except for the bizarre soft hyphen.
		}
		return false
	}
	return isPrint(r)
}

// isInGraphicList reports whether the rune is a graphic rune that IsPrint
// does not match: a space separator other than U+0020.
// Should be called only if IsPrint fails.
func isInGraphicList(r rune) bool {
	return isSpace(r)
}

// isPrint reports whether r is in category L, M, N, P or S.
func isPrint(r rune) bool

// isSpace reports whether r is in category Zs.
func isSpace(r rune) bool
