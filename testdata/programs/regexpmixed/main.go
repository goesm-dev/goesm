// Command regexpmixed matches constant patterns that goesm translates to
// RegExps (internal/lower/regexpjs.go) in a program that also needs Go's
// engine: for a pattern translation refuses, a pattern given at run time,
// a RuneReader and leftmost-longest matching. The translated patterns keep
// their RegExp for the rest.
package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// runes is a RuneReader of any runes, valid or not.
type runes struct {
	rs []rune
	i  int
}

func (r *runes) ReadRune() (rune, int, error) {
	if r.i == len(r.rs) {
		return 0, 0, io.EOF
	}
	c := r.rs[r.i]
	r.i++
	return c, 1, nil
}

var (
	word    = regexp.MustCompile(`\w+`)
	date    = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	alt     = regexp.MustCompile(`a|ab`)
	tag     = regexp.MustCompile(`<([a-z]+)>`)
	refused = regexp.MustCompile(`(a+)+b`) // exponential under backtracking
	nullRep = regexp.MustCompile(`(?:a{0,1}x?)*y`)
)

func main() {
	in := "go 2026-11-07 ab <em>aab</em> xxy ab"

	fmt.Println(word.FindAllString(in, -1))
	fmt.Println(date.FindStringSubmatch(in), date.FindAllStringSubmatchIndex(in, -1))
	fmt.Println(tag.ReplaceAllString(in, "[$1]"), string(tag.ReplaceAll([]byte(in), []byte("{$1}"))))

	// Translation refuses these: Go's engine.
	fmt.Println(refused.FindStringSubmatchIndex(strings.Repeat("a", 30)+"b"), refused.MatchString(strings.Repeat("a", 28)))
	fmt.Println(nullRep.FindAllStringIndex(in, -1))

	// A pattern given at run time: Go's engine.
	dyn := regexp.MustCompile(os.Args[0][:0] + `x+y`)
	fmt.Println(dyn.FindString(in))

	// A RuneReader reads only as far as the match needs.
	r := strings.NewReader("12 apples 345 pears")
	fmt.Println(word.FindReaderIndex(r), r.Len())
	r = strings.NewReader("on 2026-11-07, then")
	fmt.Println(date.FindReaderSubmatchIndex(r), r.Len(), date.MatchReader(strings.NewReader("1999-01-02")))

	// Leftmost-first, then leftmost-longest.
	fmt.Println(alt.FindString("ab"), alt.FindAllString("ab ab a", -1))
	longest := alt.Copy()
	longest.Longest()
	fmt.Println(longest.FindString("ab"), longest.FindAllString("ab ab a", -1), alt.FindString("ab"))

	// The translated patterns still match after Go's engine ran.
	fmt.Println(word.FindAllStringIndex("héllo wörld", -1), word.FindString(string([]byte{0xff, 'z'})))
	fmt.Println(date.NumSubexp(), date.SubexpNames(), tag.String())
	p, complete := tag.LiteralPrefix()
	fmt.Println(p, complete)

	// Go's engine checks empty-width assertions between any two runes,
	// including ones past unicode.MaxRune and negative ones.
	odd := []rune{'a', 0x7fffffff, 'b', -5, '\n', 'c', 0x10ffff, '_', 0x110000}
	for _, expr := range []string{`\b`, `\B`, `(?m)^`, `(?m)$`, `\A`, `\z`, `\b\w\b`, `(?m)^\w`} {
		re := regexp.MustCompile(os.Args[0][:0] + expr)
		var all [][]int
		for pos := 0; pos <= len(odd); pos++ {
			all = append(all, re.FindReaderIndex(&runes{rs: odd[pos:]}))
		}
		fmt.Println(expr, all, re.FindAllStringIndex("a b\nc_d\n", -1))
	}
}
