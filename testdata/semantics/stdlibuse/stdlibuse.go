// Package stdlibuse calls standard library packages that goesm compiles
// from their ordinary Go source (no hand-written TypeScript ports).
package stdlibuse

import (
	"math/bits"
	"unicode"
	"unicode/utf8"
)

func RuneCount() int { return utf8.RuneCountInString("héllo, 世界") }

func DecodeLast() []int {
	r, size := utf8.DecodeLastRuneInString("世界")
	return []int{int(r), size}
}

func Valid() []bool {
	return []bool{utf8.ValidString("ok"), utf8.ValidString("\xff"), utf8.Valid([]byte("世"))}
}

func AppendRune() string {
	return string(utf8.AppendRune([]byte("x"), '界'))
}

func Upper() string {
	var out []rune
	for _, r := range "héllo, ωorld" {
		out = append(out, unicode.ToUpper(r))
	}
	return string(out)
}

func Classes() []bool {
	return []bool{unicode.IsLetter('é'), unicode.IsDigit('٣'), unicode.IsSpace('　'), unicode.Is(unicode.Han, '世')}
}

func Bits32() []int {
	return []int{bits.OnesCount32(0xF0F0), bits.LeadingZeros32(1), bits.TrailingZeros32(8), bits.Len32(1023), int(bits.Reverse8(1)), int(bits.RotateLeft32(1, -1))}
}
