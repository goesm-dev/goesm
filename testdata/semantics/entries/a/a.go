// Package a is one of two entries of a program (TestSeveralEntries).
package a

import "example.com/sem/entries/shared"

func Add(n int) int {
	shared.Count += n
	return shared.Count
}

// Lookup returns a handle inside an any: its methods are JavaScript's.
func Lookup() any { return &shared.Box[int]{} }
