//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of unicode/utf8's utf8.go: Valid and ValidString index
// the bytes instead of reslicing, and skip ASCII one byte at a time
// rather than eight: Go reads eight bytes as one uint64 word, which goesm
// holds in a BigInt. ValidString of an ASCII string is the engine's
// regular expression test (isASCII in runtime/src/natives.ts).
package utf8

// Valid reports whether p consists entirely of valid UTF-8-encoded runes.
func Valid(p []byte) bool {
	n := len(p)
	for i := 0; i < n; {
		p0 := p[i]
		if p0 < RuneSelf {
			i++
			continue
		}
		x := first[p0]
		size := int(x & 7)
		accept := acceptRanges[x>>4]
		switch size {
		case 2:
			if n-i < 2 || p[i+1] < accept.lo || accept.hi < p[i+1] {
				return false
			}
		case 3:
			if n-i < 3 || p[i+1] < accept.lo || accept.hi < p[i+1] || p[i+2] < locb || hicb < p[i+2] {
				return false
			}
		case 4:
			if n-i < 4 || p[i+1] < accept.lo || accept.hi < p[i+1] || p[i+2] < locb || hicb < p[i+2] || p[i+3] < locb || hicb < p[i+3] {
				return false
			}
		default:
			return false // illegal starter byte
		}
		i += size
	}
	return true
}

// ValidString reports whether s consists entirely of valid UTF-8-encoded runes.
func ValidString(s string) bool {
	if isASCII(s) {
		return true
	}
	n := len(s)
	for i := 0; i < n; {
		s0 := s[i]
		if s0 < RuneSelf {
			i++
			continue
		}
		x := first[s0]
		size := int(x & 7)
		accept := acceptRanges[x>>4]
		switch size {
		case 2:
			if n-i < 2 || s[i+1] < accept.lo || accept.hi < s[i+1] {
				return false
			}
		case 3:
			if n-i < 3 || s[i+1] < accept.lo || accept.hi < s[i+1] || s[i+2] < locb || hicb < s[i+2] {
				return false
			}
		case 4:
			if n-i < 4 || s[i+1] < accept.lo || accept.hi < s[i+1] || s[i+2] < locb || hicb < s[i+2] || s[i+3] < locb || hicb < s[i+3] {
				return false
			}
		default:
			return false // illegal starter byte
		}
		i += size
	}
	return true
}

func isASCII(s string) bool
