package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"
)

// Interface values of types with methods called through interfaces are
// instances of per-type box classes (internal/lower/box.go); a struct's box
// is the struct object itself.

type Shape interface {
	Area() float64
	Scale(f float64) Shape
}

type rect struct{ w, h float64 }

func (r rect) Area() float64         { return r.w * r.h }
func (r rect) Scale(f float64) Shape { return rect{r.w * f, r.h * f} }

// A struct with fields named t and v keeps a box holding the value.
type tv struct{ t, v float64 }

func (x tv) Area() float64         { return x.t * x.v }
func (x tv) Scale(f float64) Shape { return tv{x.t * f, x.v * f} }

// A value receiver changed by the method: the interface's value stays.
type counter struct{ n int }

func (c counter) Bump() int { c.n++; return c.n }

type bumper interface{ Bump() int }

// Pointer receivers, called on a nil pointer.
type node struct{ next *node }

func (n *node) Len() int {
	if n == nil {
		return 0
	}
	return 1 + n.next.Len()
}

type lener interface{ Len() int }

// A non-struct type and a pointer to one.
type celsius float64

func (c celsius) String() string { return fmt.Sprintf("%.1fC", float64(c)) }

type tally int

func (t *tally) Add(n int) int { *t += tally(n); return int(*t) }

type adder interface{ Add(n int) int }

// Embedding: promoted methods.
type square struct {
	rect
	name string
}

// Unexported interface methods have package-qualified keys.
type hidden interface{ secret(a, b, c, d, e int) int }

type keeper struct{ k int }

func (k keeper) secret(a, b, c, d, e int) int { return k.k + a + b + c + d + e }

// A generic type in an interface: a plain Iface.
type box[T any] struct{ x T }

func (b box[T]) String() string { return fmt.Sprint("box ", b.x) }

// Exported fields, set through reflection.
type Pt struct{ X, Y float64 }

func (p Pt) Area() float64         { return p.X * p.Y }
func (p Pt) Scale(f float64) Shape { return Pt{p.X * f, p.Y * f} }

type empty struct{}

func (empty) Area() float64         { return 1 }
func (empty) Scale(f float64) Shape { return empty{} }

type myErr struct{ code int }

func (e myErr) Error() string { return fmt.Sprintf("code %d", e.code) }

func sum(ss []Shape) float64 {
	t := 0.0
	for _, s := range ss {
		t += s.Scale(2).Area()
	}
	return t
}

func generic[T Shape](x T) float64 { return x.Area() + x.Scale(3).Area() }

func main() {
	r := rect{2, 3}
	ss := []Shape{r, tv{1, 2}, empty{}, square{rect{1, 1}, "sq"}, &rect{4, 5}}
	fmt.Println(sum(ss))
	r.w = 100 // the boxed copy is unchanged
	fmt.Println(ss[0].Area(), ss[0].(rect).w)
	got := ss[0].(rect)
	got.w = 7
	fmt.Println(ss[0].(rect).w, got.w)
	fmt.Println(ss[0] == Shape(rect{2, 3}), ss[1] == Shape(tv{1, 2}), ss[0] == ss[1])
	m := map[Shape]int{rect{1, 2}: 1, tv{1, 2}: 2}
	fmt.Println(m[rect{1, 2}], m[tv{1, 2}], m[Shape(empty{})])

	var b bumper = counter{5}
	fmt.Println(b.Bump(), b.Bump(), b.(counter).n)

	var l lener = (*node)(nil)
	fmt.Println(l.Len(), lener(&node{&node{}}).Len())

	var s fmt.Stringer = celsius(21.5)
	fmt.Println(s.String(), s, []any{celsius(3)})
	var t tally
	var a adder = &t
	a.Add(2)
	fmt.Println(a.Add(3), t)

	var h hidden = keeper{1}
	fmt.Println(h.secret(1, 2, 3, 4, 5))

	var g fmt.Stringer = box[int]{4}
	fmt.Println(g.String(), g)

	fmt.Println(generic(rect{1, 2}), generic[Shape](tv{2, 2}))

	f := ss[0].Area
	fmt.Println(f())

	var err error = myErr{3}
	var me myErr
	fmt.Println(errors.As(fmt.Errorf("wrap: %w", err), &me), me.code, err)

	v := reflect.ValueOf(ss[0])
	fmt.Println(v.Type(), v.Field(0).Float(), v.Interface().(Shape).Area())
	rv := reflect.New(reflect.TypeOf(Pt{})).Elem()
	rv.Field(0).SetFloat(9)
	rv.Field(1).SetFloat(2)
	fmt.Println(rv.Interface().(Shape).Scale(0.5).Area())

	js, _ := json.Marshal(map[string]any{"r": struct{ W float64 }{1}, "d": 2 * time.Second})
	fmt.Println(string(js))
	fmt.Printf("%v %+v %T\n", ss[0], ss[1], ss[3])

	shapes := []Shape{rect{3, 3}, tv{1, 1}, rect{1, 2}}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Area() < shapes[j].Area() })
	fmt.Println(shapes)
	fmt.Println(time.Duration(1500) * time.Millisecond)
}
