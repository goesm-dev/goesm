// Package items keeps its values in a value of its own interface type.
package items

import "fmt"

type namer interface{ Name() string }

type item struct{ id int }

func (it *item) Name() string { return "item" }

// Dump is never called: no value of an interface type that reaches
// JavaScript holds an *item.
func (it *item) Dump() string { return fmt.Sprintf("%#v %v", *it, []any{it.id, 1.5, "x"}) }

var current namer = &item{id: 1}

// Name returns the name of the current item.
func Name() string { return current.Name() }
