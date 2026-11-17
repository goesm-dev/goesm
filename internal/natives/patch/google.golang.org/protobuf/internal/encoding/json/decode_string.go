package json

import "unicode/utf8"

// indexNeedEscapeInBytes returns the index of the character that needs
// escaping. If no characters need escaping, this returns the input length.
//
// The original converts b to a string with unsafe.String and scans that.
// goesm's strings are JavaScript strings, which cannot alias the bytes, so
// the conversion copies all of b: parseString calls this with the rest of
// the input, and decoding took time quadratic in its length. This scans b
// as indexNeedEscapeInString scans the string, rune by rune, stopping at
// the same characters (utf8.DecodeRune decodes as range does).
func indexNeedEscapeInBytes(b []byte) int {
	for i := 0; i < len(b); {
		if c := b[i]; c < utf8.RuneSelf {
			if c < ' ' || c == '\\' || c == '"' {
				return i
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError {
			return i
		}
		i += n
	}
	return len(b)
}
