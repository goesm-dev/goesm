// Anchored patterns with quantifiers: the backtracker reads the text before
// the match through the bounds check idiom uint(pos-1) < uint(len(s)), with
// pos 0 (a regression: it matched nothing).
package main

import (
	"fmt"
	"regexp"
)

//go:noinline
func inRange(i, n int) bool { return uint(i) < uint(n) }

func main() {
	for _, p := range []string{`^ab*`, `^a *`, `^ab+`, `^ab?`, `^(?:ab)*`, `^\[x\]\s*`, `x\s*`, `a+b`, `^\[x\]\s`, `(?m)^b`, `\bc$`} {
		re := regexp.MustCompile(p)
		fmt.Println(p, re.FindStringIndex("ab c"), re.MatchString("ab c"), re.FindStringIndex("[x] done"))
	}
	task := regexp.MustCompile(`^\[([\sxX])\]\s*`)
	fmt.Printf("%q\n", task.FindStringSubmatch("[x] done"))
	fmt.Println(regexp.MustCompile(`^(\w+)\s*=\s*(\d+)$`).FindAllStringSubmatch("width = 42", -1))
	fmt.Println(inRange(-1, 4), inRange(0, 4), inRange(4, 4), inRange(3, 4))
}
