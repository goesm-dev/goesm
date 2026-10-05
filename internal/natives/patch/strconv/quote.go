//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strconv's quote.go: Quote of a string of printable
// ASCII without a quote or a backslash, which it returns unchanged between
// quotes, concatenates instead of appending to a []byte.
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
