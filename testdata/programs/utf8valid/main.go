// Command utf8valid checks utf8.Valid and utf8.ValidString, which goesm
// patches, on ASCII, every class of multi-byte sequence and the ways a
// sequence can be invalid, at every position in a longer string.
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func main() {
	cases := []string{
		"", "a", "hello, world", strings.Repeat("ascii ", 20),
		"é", "日本語", "😀", "߿ࠀ￿\U00010000\U0010ffff",
		"\x80", "\xc0\x80", "\xc1\xbf", "\xc2", "\xc2\x7f", "\xe0\x80\x80", "\xe0\x9f\xbf",
		"\xed\xa0\x80", "\xed\x9f\xbf", "\xef\xbf", "\xf0\x8f\xbf\xbf", "\xf4\x90\x80\x80",
		"\xf4\x8f\xbf\xbf", "\xf5\x80\x80\x80", "\xff", "\xfe", "a\xe3\x81",
	}
	pad := strings.Repeat("x", 17)
	for _, c := range cases {
		for _, s := range []string{c, pad + c, c + pad, pad + c + pad} {
			v, vs := utf8.Valid([]byte(s)), utf8.ValidString(s)
			fmt.Print(v, vs, " ")
			if v != vs {
				fmt.Printf("mismatch %q\n", s)
			}
		}
		fmt.Println()
	}
}
