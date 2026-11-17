// Command regexpwait matches patterns that goesm translates to RegExps and
// patterns Go's engines match (one-pass, backtracking and the NFA) in
// strings and byte slices, in a program that also searches a RuneReader
// whose ReadRune waits on a channel. Only the methods that read a
// RuneReader wait: the others, and the functions calling them, stay
// synchronous (TestAsyncStaysLocal).
package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	word    = regexp.MustCompile(`^[a-z]+$`)         // a RegExp
	number  = regexp.MustCompile(`-?\d+\.?\d*`)      // a RegExp too
	onePass = regexp.MustCompile(`^(?:(a)|b)+c$`)    // Go's one-pass engine
	nested  = regexp.MustCompile(`(x+)+y`)           // backtracking, or the NFA on long input
	pair    = regexp.MustCompile(`(\w+)=(\w*)(;|$)`) // a RegExp
	// Leftmost-longest (main calls Longest): Go's engines.
	longNested = regexp.MustCompile(`(x+)+y`)
	longPair   = regexp.MustCompile(`(\w+)=(\w*)(;|$)`)
)

// waiting is a RuneReader whose runes come from another goroutine.
type waiting struct{ ch chan rune }

func newWaiting(s string) *waiting {
	w := &waiting{ch: make(chan rune)}
	go func() {
		for _, r := range s {
			w.ch <- r
		}
		close(w.ch)
	}()
	return w
}

func (w *waiting) ReadRune() (rune, int, error) {
	r, ok := <-w.ch
	if !ok {
		return 0, 0, io.EOF
	}
	return r, utf8.RuneLen(r), nil
}

// text matches strings and byte slices only.
func text(s string) string {
	var b strings.Builder
	fmt.Fprintln(&b, word.MatchString(s), number.FindAllString(s, -1), number.ReplaceAllString(s, "<$0>"))
	fmt.Fprintln(&b, onePass.FindStringSubmatch(s), onePass.MatchString(s), onePass.Match([]byte(s)))
	fmt.Fprintln(&b, nested.FindStringSubmatchIndex(s), nested.FindAllIndex([]byte(s), -1), nested.Split(s, -1))
	fmt.Fprintln(&b, pair.FindAllStringSubmatch(s, -1), pair.ReplaceAllString(s, "$2=$1$3"))
	fmt.Fprintln(&b, longNested.FindString(s), longNested.ReplaceAllLiteralString(s, "_"), longPair.FindAllStringSubmatchIndex(s, -1))
	return b.String()
}

// longInput matches an input the backtracker leaves to the NFA.
func longInput() string {
	s := strings.Repeat("x", 100000) + "y" + strings.Repeat("x", 50000)
	loc := nested.FindStringSubmatchIndex(s)
	return fmt.Sprint(loc, len(nested.FindAllString(s, -1)), onePass.MatchString(strings.Repeat("ab", 50000)+"c"))
}

// readers searches RuneReaders that wait.
func readers(s string) string {
	return fmt.Sprintln(
		word.MatchReader(newWaiting(s)),
		number.FindReaderIndex(newWaiting(s)),
		onePass.FindReaderSubmatchIndex(newWaiting(s)),
		nested.MatchReader(newWaiting(s)),
		longNested.FindReaderIndex(newWaiting(s)),
		longPair.FindReaderSubmatchIndex(newWaiting(s)),
		pair.FindReaderSubmatchIndex(newWaiting(s)),
	)
}

func main() {
	longNested.Longest()
	longPair.Longest()
	for _, s := range []string{"abc", "aabac", "abd", "x1.5 -2 3. 4.25x", "xxxy and xy, xxx", "a=1;b=;c=xyz", ""} {
		fmt.Printf("%q\n%s%s", s, text(s), readers(s))
	}
	fmt.Println(longInput())
	m, err := regexp.MatchReader(`b+c`, newWaiting("abbbc"))
	fmt.Println(m, err)
}
