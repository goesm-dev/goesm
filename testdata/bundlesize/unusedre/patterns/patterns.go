// Package patterns holds compiled regular expressions in package
// variables, next to a function that uses none of them (see TestBundleSize).
package patterns

import "regexp"

var (
	// Matched with a RegExp.
	word = regexp.MustCompile(`^[a-z]+\d*$`)
	// Backtracking could take more than quadratic time: Go's engines.
	nested = regexp.MustCompile(`(a+)+b`)
)

func Word(s string) bool { return word.MatchString(s) }

func Nested(s string) bool { return nested.MatchString(s) }

func Add(a, b int) int { return a + b }
