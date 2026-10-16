// Package registry boxes a handle into an any for the jsexport entry to
// forward.
package registry

// Item is a handle no exported signature names.
type Item struct{ n int }

func (i *Item) N() int { return i.n }

// Get returns an Item inside an any.
func Get() any { return &Item{n: 3} }
