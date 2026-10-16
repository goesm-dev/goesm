// Package b is the other entry (TestSeveralEntries).
package b

import "example.com/sem/entries/shared"

func Count() int { return shared.Count }

// Wait blocks, so it is async for JavaScript.
func Wait() int {
	ch := make(chan int)
	go func() { ch <- shared.Count }()
	return <-ch
}
