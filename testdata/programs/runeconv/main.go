// Conversions between strings and runes, and ranging over strings, on
// valid and invalid UTF-8: truncated and overlong sequences, encoded
// surrogates, runes past the last code point, and inputs long enough to be
// converted in several pieces.
package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var inputs = []string{
	"",
	"ascii only",
	"héllo, 世界 🙂",
	"\xff",
	"a\x80b",
	"\xc3",             // truncated 2-byte sequence
	"\xe4\xb8",         // truncated 3-byte sequence
	"\xf0\x9f\x99",     // truncated 4-byte sequence
	"\xc0\x80",         // overlong NUL
	"\xe0\x80\xaf",     // overlong '/'
	"\xed\xa0\x80",     // encoded surrogate U+D800
	"\xf4\x90\x80\x80", // past U+10FFFF
	"\xf4\x8f\xbf\xbf", // U+10FFFF
	"\xef\xbf\xbd",     // U+FFFD itself
	"x\xe4\xb8\x96\xe7\x95y",
}

func main() {
	for _, s := range inputs {
		rs := []rune(s)
		fmt.Printf("%q runes=%U count=%d back=%q\n", s, rs, utf8.RuneCountInString(s), string(rs))
		var parts []string
		for i, r := range s {
			parts = append(parts, fmt.Sprintf("%d:%U", i, r))
		}
		fmt.Println("  range", strings.Join(parts, " "))
		n := 0
		for range s {
			n++
		}
		var only []int
		for i := range s {
			only = append(only, i)
		}
		fmt.Println("  keys", n, only, fmt.Sprintf("[%-8s]", s) == "["+s+strings.Repeat(" ", max(0, 8-utf8.RuneCountInString(s)))+"]")
		fmt.Printf("  split %q\n", strings.Split(s, ""))
	}

	// Runes that are not valid code points become U+FFFD.
	bad := []rune{'a', -1, 0xD800, 0xDFFF, 0x10FFFF, 0x110000, 0x7fffffff, '世', 0}
	fmt.Printf("%q\n", string(bad))
	for _, r := range bad {
		fmt.Printf("%q ", string(r))
	}
	fmt.Println()

	// Long inputs, converted in pieces.
	var b strings.Builder
	for i := 0; i < 30000; i++ {
		switch i % 4 {
		case 0:
			b.WriteByte(byte('a' + i%26))
		case 1:
			b.WriteRune(rune(0x4e00 + i%500))
		case 2:
			b.WriteRune(rune(0x1f600 + i%50))
		default:
			b.WriteString("\xe4")
		}
	}
	long := b.String()
	lr := []rune(long)
	back := string(lr)
	sum := 0
	for i, r := range long {
		sum += i ^ int(r)
	}
	fmt.Println(len(long), len(lr), len(back), utf8.ValidString(back), sum, back[len(back)-12:] == string(lr[len(lr)-4:]))
	big := make([]rune, 20000)
	for i := range big {
		big[i] = rune(0x10000 + i*37)
	}
	bs := string(big)
	fmt.Println(len(bs), utf8.RuneCountInString(bs), []rune(bs)[19999] == big[19999])

	// Byte offsets of matches in text that is not ASCII.
	re := regexp.MustCompile(`[a-z]+`)
	fmt.Println(re.FindAllStringIndex("ünïcödé \xffabc 世界xyz", -1))
}
