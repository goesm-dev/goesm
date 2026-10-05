// Package reachtree is a syntax tree whose Dump method only Dump calls
// through the interface (see ../methodreach).
package reachtree

import "fmt"

type Node interface {
	Kind() string
	Dump(level int) string
}

type Elem struct{ Kids []Node }

func (e *Elem) Kind() string { return "elem" }

func (e *Elem) Dump(level int) string {
	s := fmt.Sprint(level)
	for _, k := range e.Kids {
		s += k.Dump(level + 1)
	}
	return s
}

type Text string

func (t Text) Kind() string { return "text" }

func (t Text) Dump(level int) string { return string(t) }

// Wrap embeds Node: its promoted Dump is in its table too.
type Wrap struct{ Node }
