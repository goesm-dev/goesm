// Package regexgo matches a pattern known at compile time that goesm does
// not translate to a RegExp, since backtracking it could take quadratic
// time, so regexp keeps Go's engines, without unicode's category and
// script tables, which regexp/syntax needs only for \p (see
// TestBundleSize).
package regexgo

import "regexp"

var addr = regexp.MustCompile(`(\w+)@(\w+)`)

func User(s string) string {
	m := addr.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}
