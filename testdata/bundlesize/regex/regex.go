// Package regex matches a pattern put together from package variables,
// without a Unicode class, and builds an example.com/tree, whose Dump it
// does not call (see TestBundleSize).
package regex

import (
	"regexp"

	"example.com/tree"
)

var (
	name = `[A-Za-z][A-Za-z0-9-]*`
	tag  = regexp.MustCompile("^<" + name + `\s*/?>`)
)

func Parse(s string) string {
	if !tag.MatchString(s) {
		return ""
	}
	var n tree.Node = &tree.Elem{Name: s}
	return n.Kind()
}
