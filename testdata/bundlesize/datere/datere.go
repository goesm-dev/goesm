// Package datere parses a date with a regular expression that goesm
// translates to a RegExp, so regexp's parser and engines are left out (see
// TestBundleSize).
package datere

import (
	"regexp"
	"strconv"
)

var date = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)

func Parse(s string) []int {
	m := date.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	var ymd []int
	for _, p := range m[1:] {
		n, _ := strconv.Atoi(p)
		ymd = append(ymd, n)
	}
	return ymd
}
