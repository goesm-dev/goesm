// Package shared is state that two entry packages of one program share.
package shared

// Count is changed through one entry and read through the other.
var Count int

// Box is a generic handle; only entry a stores one in an any.
type Box[T any] struct{ v T }

func (b *Box[T]) Get() T  { return b.v }
func (b *Box[T]) Set(v T) { b.v = v }
