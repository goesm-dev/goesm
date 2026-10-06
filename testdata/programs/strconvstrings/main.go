// Command strconvstrings checks the parts of strconv and strings that goesm
// implements apart (Atoi's slow path, TrimSpace's Unicode white space,
// Split with an empty separator) against native Go.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

func main() {
	for _, s := range []string{
		"", "+", "-", "0", "-0", "+0", "123", "-123", "1x", "x", "1_000", "0x10", "０", " 12", "12 ",
		"9007199254740991", "-9007199254740991", "123456789012345678", "1234567890123456789", "-1234567890123456789",
		"9223372036854775807", "9223372036854775808", "-9223372036854775808", "-9223372036854775809",
		"18446744073709551615", "18446744073709551616", "-18446744073709551616", "99999999999999999999x",
		"9999999999999999999x", "x99999999999999999999", "0000000000000000000000000000001", "-0000000000000000000000009223372036854775808",
		"+0000000000000000000000009223372036854775807", "++1", "--1", "+-1", "12345678901234567890123456789",
	} {
		// goesm's int is a JavaScript number, exact only within ±(2⁵³-1).
		n, err := strconv.Atoi(s)
		if n > 1<<53-1 || n < -(1<<53-1) {
			fmt.Printf("Atoi(%q) = beyond 2^53, %v\n", s, err)
			continue
		}
		fmt.Printf("Atoi(%q) = %d, %v\n", s, n, err)
	}

	// The runes TrimSpace removes, at either end.
	var front, back []rune
	for r := rune(0); r <= 0x10FFFF+1; r++ {
		if strings.TrimSpace(string(r)+"a") == "a" {
			front = append(front, r)
		}
		if strings.TrimSpace("a"+string(r)) == "a" {
			back = append(back, r)
		}
	}
	fmt.Printf("%U\n%U\n", front, back)
	for _, s := range []string{
		"", " ", "　", " 　 x 　 ", "\u0085 x  ", "\xe3\x80", "x\x80\x80", "\xe3\x80\x80\xe3\x80",
		"\xc2\x85", "\xc2", "\x85", "a\xe2\x80\xa8", "\xe2\x80\xa8\xe2\x80\xa8b\t", "\xf0\x80\x80\x80 ", " \xff ",
	} {
		fmt.Printf("TrimSpace(%q) = %q\n", s, strings.TrimSpace(s))
	}

	for _, s := range []string{"", "a", "abc", "日本語", "a\xffb", "\xe3\x80", "\xed\xa0\x80", "😀x", "\xf4\x90\x80\x80"} {
		p := strings.Split(s, "")
		fmt.Printf("Split(%q, \"\") = %q %d %v\n", s, p, len(p), p == nil)
	}
}
