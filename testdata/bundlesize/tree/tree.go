// Package tree is a syntax tree whose Dump method only Dump calls through
// the interface, as goldmark's (see TestBundleSize).
package tree

import (
	"fmt"
	"strings"
)

type Node interface {
	Kind() string
	Dump(level int)
}

type Elem struct {
	Name string
	Kids []Node
}

func (e *Elem) Kind() string { return "elem" }

func (e *Elem) Dump(level int) {
	fmt.Printf("%s%s\n", strings.Repeat(" ", level), e.Name)
	for _, k := range e.Kids {
		k.Dump(level + 1)
	}
}
