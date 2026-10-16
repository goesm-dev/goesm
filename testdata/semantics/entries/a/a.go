// Package a is one of two entries of a program (TestSeveralEntries).
package a

import (
	"strconv"

	"example.com/sem/entries/shared"
)

func Add(n int) int {
	shared.Count += n
	return shared.Count
}

// Lookup returns a handle inside an any: its methods are JavaScript's.
func Lookup() any { return &shared.Box[int]{} }

// Counter is a type of an entry that the other entry uses.
type Counter struct{ N int }

func (c *Counter) Inc() int {
	c.N++
	return c.N
}

// Label takes a string and returns a slice, which the wrapper converts.
func (c *Counter) Label(prefix string) []string { return []string{prefix + strconv.Itoa(c.N)} }

func NewCounter() *Counter { return &Counter{N: 4} }

// Double doubles the elements of ns in place.
func Double(ns []int) {
	for i := range ns {
		ns[i] *= 2
	}
}

// Ages is a map, which the wrapper converts to a JavaScript object.
func Ages() map[string]int { return map[string]int{"gopher": 16} }
