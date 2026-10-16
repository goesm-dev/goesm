// Package registry boxes a handle into an any for the jsexport entry to
// forward.
package registry

// Item is a handle no exported signature names.
type Item struct{ n int }

func (i *Item) N() int { return i.n }

// Get returns an Item inside an any.
func Get() any { return &Item{n: 3} }

// Boxed and Held are handles generic code boxes: their instantiations name
// them, the conversions only a type parameter.
type Boxed struct{ n int }

func (b *Boxed) N() int { return b.n }

type Held struct{ n int }

func (h *Held) N() int { return h.n }

func box[T any](v T) any { return v }

// Wrap passes its type parameter on to box.
func Wrap[T any](v T) any { return box(v) }

// Holder boxes its value in a method.
type Holder[T any] struct{ v T }

func (h Holder[T]) Any() any { return h.v }

func GetBoxed() any { return Wrap(&Boxed{n: 4}) }

func GetHeld() any { return Holder[*Held]{&Held{n: 5}}.Any() }

// Listed and Paired are handles inside a slice and a struct boxed into an
// any: JavaScript gets the slice as an array, the struct as an object.
type Listed struct{ n int }

func (l *Listed) N() int { return l.n }

type Paired struct{ n int }

func (p *Paired) N() int { return p.n }

type Pair struct{ First *Paired }

func GetList() any { return []*Listed{{n: 6}, {n: 7}} }

func GetPair() any { return Pair{&Paired{n: 8}} }
