// Command unicodecase prints unicode.ToUpper and ToLower, which goesm's
// patch implements with the engine's case mapping, over the scripts with
// case, and strings.ToUpper and ToLower of strings beyond ASCII.
package main

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
)

func main() {
	// Unicode 16 and earlier: case pairs added later depend on the
	// engine's Unicode version.
	ranges := [][2]rune{{0, 0xa7cd}, {0xa7d6, 0xffff}, {0x10400, 0x16e9f}, {0x16ed4, 0x1e943}, {0x10fff0, 0x110010}, {-2, -1}}
	h := fnv.New64a()
	upper, lower := 0, 0
	for _, rg := range ranges {
		for r := rg[0]; r <= rg[1]; r++ {
			u, l := unicode.ToUpper(r), unicode.ToLower(r)
			if u != r {
				upper++
			}
			if l != r {
				lower++
			}
			fmt.Fprintf(h, "%d %d %d\n", r, u, l)
		}
	}
	fmt.Printf("%d %d %x\n", upper, lower, h.Sum64())
	for _, r := range []rune{'İ', 'ß', 'ŉ', 'ᾳ', 'ᾀ', 'ᾏ', 'ῼ', 'Σ', 'ς', 'ǅ', 'ﬀ', 'Ω', '𐐀', '𞤢', 0xd800} {
		fmt.Printf("%U %U %U\n", r, unicode.ToUpper(r), unicode.ToLower(r))
	}
	for _, s := range []string{"İstanbul", "Straße", "ΟΔΥΣΣΕΥΣ", "ᾳ ᾼ", "Ǆemal", "ｂｅｅ ＢＥＥ"} {
		fmt.Println(strings.ToUpper(s), strings.ToLower(s))
	}
}
