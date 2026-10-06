// Package atoi parses a number with strconv.Atoi and only compares the
// error with nil, so NumError's Error method, with strconv's quoting and
// its Unicode tables, is left out (see TestBundleSize).
package atoi

import "strconv"

func Parse(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}
