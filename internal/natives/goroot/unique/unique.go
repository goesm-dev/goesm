//go:build goesm

// Package unique is goesm's replacement for the standard library's unique
// package. The gc implementation keeps a concurrent, weakly referenced map
// per type, reached through the type's runtime type descriptor and
// unsafe.Pointer. Here one Go map holds the canonical copies; its keys pair
// the value with its static type, since equal values of different types
// must not share a handle. Canonical values are never freed.
package unique

// Handle is a globally unique identity for some value of type T.
//
// Two handles compare equal exactly if the two values used to create the
// handles would have also compared equal. The comparison of two handles is
// trivial and typically much more efficient than comparing the values used
// to create them.
type Handle[T comparable] struct {
	value *T
}

// Value returns a shallow copy of the T value that produced the Handle.
// Value is safe for concurrent use by multiple goroutines.
func (h Handle[T]) Value() T {
	return *h.value
}

// Make returns a globally unique handle for a value of type T. Handles
// are equal if and only if the values used to produce them are equal.
// Make is safe for concurrent use by multiple goroutines.
func Make[T comparable](value T) Handle[T] {
	key := [2]any{(*T)(nil), value}
	if p, ok := handles[key]; ok {
		return Handle[T]{p.(*T)}
	}
	p := new(T)
	*p = value
	handles[key] = p
	return Handle[T]{p}
}

var handles = map[[2]any]any{}
