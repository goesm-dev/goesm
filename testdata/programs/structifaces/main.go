// Command structifaces stores values of named struct types in interfaces,
// which goesm does without a box: the struct object is the interface value.
// The program checks that the dynamic type and value semantics stay Go's
// wherever such values meet values the runtime boxes.
package main

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync/atomic"
)

type A struct{ X, Y int }

// B shares A's underlying type, so (*B)(&a) is the same object as a.
type B A

type Shape interface{ Area() float64 }

type rect struct{ w, h float64 }

func (r rect) Area() float64 { return r.w * r.h }

// Grow changes its copy of the receiver only.
func (r rect) Grow() rect {
	r.w *= 2
	return r
}

type square struct {
	rect // promotes Area
	name string
}

type codeError struct{ code int }

func (e codeError) Error() string { return fmt.Sprint("code ", e.code) }

// t and v are the names of an interface value's parts in JS: the type keeps
// its box.
type tv struct{ t, v int }

func box[T any](x T) any { return x }

func find() error { return codeError{404} }

func main() {
	a := &A{1, 2}
	b := (*B)(a)
	b.Y = 3
	var i any = *b
	_, isB := i.(B)
	_, isA := i.(A)
	fmt.Println(isB, isA, i, a.Y)
	fmt.Println(any(*a) == any(A{1, 3}), any(*a) == i, i == any(B{1, 3}))

	// A copy goes into the interface; the variable stays apart.
	r := rect{2, 3}
	var s Shape = r
	r.w = 100
	fmt.Println(s.Area(), s.(rect).Grow().Area(), s.Area())
	got := s.(rect)
	got.h = 0
	fmt.Println(s.Area(), got.Area())

	// Promoted methods, type switches and reflect.
	var sq Shape = square{rect{1, 4}, "sq"}
	switch v := sq.(type) {
	case rect:
		fmt.Println("rect", v)
	case square:
		fmt.Println("square", v.name, v.Area())
	}
	fmt.Println(reflect.TypeOf(sq), reflect.ValueOf(sq).Field(1))

	// Values boxed by the runtime equal values that are their own boxes.
	rv := reflect.New(reflect.TypeOf(rect{})).Elem()
	rv.Field(0).SetFloat(2)
	rv.Field(1).SetFloat(3)
	fmt.Println(rv.Interface() == any(rect{2, 3}), rv.Interface() == s)

	// Map keys and sorting through interfaces.
	m := map[any]int{rect{1, 1}: 1, A{1, 1}: 2, B{1, 1}: 3}
	m[rect{1, 1}]++
	fmt.Println(len(m), m[rect{1, 1}], m[A{1, 1}], m[B{1, 1}])
	shapes := []Shape{rect{3, 3}, square{rect{1, 1}, "a"}, rect{2, 1}}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Area() < shapes[j].Area() })
	fmt.Println(shapes)

	// Errors.
	err := fmt.Errorf("wrapped: %w", find())
	var ce codeError
	fmt.Println(errors.As(err, &ce), ce.code, errors.Is(err, codeError{404}), err)

	// Generic code and atomic.Value.
	fmt.Println(box(rect{1, 2}) == any(rect{1, 2}), box(B{5, 6}).(B).Y, box(tv{1, 2}))
	var av atomic.Value
	av.Store(rect{1, 1})
	fmt.Println(av.CompareAndSwap(rect{1, 1}, rect{2, 2}), av.Load())
	var tvi any = tv{7, 8}
	fmt.Println(tvi.(tv).t, tvi.(tv).v, tvi)
}
