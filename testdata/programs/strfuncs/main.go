// Command strfuncs prints the results of the strings and strconv functions
// goesm's patches implement with the engine's string operations, at the
// inputs where those differ from Go's byte-wise definitions.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

func main() {
	inputs := []string{
		"", " ", "\t\n\v\f\r ", "x", "  x  ", "\tsku = A-1 \n", "a b",
		"\xa0x\xa0", " \xa0 ", "\u00a0x\u00a0", "\u0085x\u0085", " \u2003x\u3000 ", "\ufeffx",
		"\xffx\xff", " \xc2 ", "日本 ", " 日本", "\x1cx\x1f",
	}
	for _, s := range inputs {
		t := strings.TrimSpace(s)
		c := strings.Clone(s)
		fmt.Printf("%q -> %q %v %s %s\n", s, t, c == s, strconv.Quote(s), strconv.Quote(t))
	}
	for _, s := range []string{"12", "x1", "99999999999999999999", "", " 1"} {
		_, err := strconv.Atoi(s)
		fmt.Println(s, err)
	}
	for _, s := range []string{"plain", `q"uote`, `back\slash`, "~ !", "\x7f", "é"} {
		fmt.Println(strconv.Quote(s), strconv.QuoteToASCII(s))
	}
}
