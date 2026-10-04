//go:build goesm

// goesm's patch of package internal/abi's escape analysis hints: gc
// implements them in the compiler, and goesm has no stack to escape from.
package abi

// EscapeNonString forces v to be on the heap, if v contains a
// non-string pointer.
func EscapeNonString[T any](v T) {}

// EscapeToResultNonString returns v.
func EscapeToResultNonString[T any](v T) T { return v }
