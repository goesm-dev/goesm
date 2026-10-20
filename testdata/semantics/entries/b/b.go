// Package b is the other entry (TestSeveralEntries). It imports entry a,
// whose functions it calls as Go functions, not through the wrappers
// JavaScript calls them by.
package b

import (
	"example.com/sem/entries/a"
	"example.com/sem/entries/shared"
)

func Count() int { return shared.Count }

// Wait blocks, so it is async for JavaScript.
func Wait() int {
	ch := make(chan int)
	go func() { ch <- shared.Count }()
	return <-ch
}

// AddTwice calls entry a with a slice, which its wrapper would expect as a
// JavaScript array.
func AddTwice(ns []int) int {
	c := &a.Counter{}
	for _, n := range ns {
		a.Add(n)
		c.Inc()
	}
	return shared.Count*10 + c.N
}

// Doubled sees the elements a.Double changed: the slice is shared, as Go
// passes it, not converted from and to JavaScript.
func Doubled() int {
	s := []int{1, 2}
	a.Double(s)
	return s[0] + s[1]
}

// Age reads the map a.Ages returns as a Go map.
func Age() int { return a.Ages()["gopher"] }

// Labelled calls a method of entry a as Go code.
func Labelled() string {
	c := &a.Counter{}
	c.Inc()
	return c.Label("b")[0]
}

// Summed calls entry a's Each with a function that does not block, through
// its synchronous clone.
func Summed() int {
	s := 0
	a.Each([]int{1, 2, 3}, func(n int) { s += n })
	return s
}

// Key takes a handle of entry a's type, whose methods b's module gives it
// once, although *a.Counter appears both here and in Counter.Copy.
func Key(c *a.Counter) int { return c.N }
