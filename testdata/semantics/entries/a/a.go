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

// Copy returns a handle of the type it is called on.
func (c *Counter) Copy() *Counter { d := *c; return &d }

// Double doubles the elements of ns in place.
func Double(ns []int) {
	for i := range ns {
		ns[i] *= 2
	}
}

// Ages is a map, which the wrapper converts to a JavaScript object.
func Ages() map[string]int { return map[string]int{"gopher": 16} }

// Each blocks only if f does: a call with an f that does not block calls its
// synchronous clone.
func Each(ns []int, f func(int)) {
	for _, n := range ns {
		f(n)
	}
}

// Relay passes Each a function that blocks.
func Relay() int {
	ch := make(chan int, 1)
	s := 0
	Each([]int{5}, func(n int) {
		ch <- n
		s += <-ch
	})
	return s
}
