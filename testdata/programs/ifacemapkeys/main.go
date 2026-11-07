// Command ifacemapkeys checks maps whose interface keys hold pointers and
// channels, which the runtime enters under the object itself: two pointer
// types of one object stay two keys, through sets, lookups, deletes and
// clear, and sync.Map and reflect.Type keys work.
package main

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

type A struct{ n int }
type B A

type empty struct{}

func main() {
	a := &A{1}
	b := (*B)(a)
	m := map[any]string{}
	m[a] = "a"
	m[b] = "b"
	fmt.Println(len(m), m[a], m[b])
	delete(m, a)
	v, ok := m[a]
	fmt.Println(len(m), v == "", ok, m[b])
	m[a] = "a2"
	m[b] = "b2"
	fmt.Println(len(m), m[a], m[b])
	delete(m, b)
	fmt.Println(len(m), m[a], m[b] == "")
	m[b] = "b3"
	delete(m, a)
	m[a] = "a3"
	fmt.Println(len(m), m[a], m[b])

	c, c2 := make(chan int), make(chan int)
	m[c] = "c"
	m[c2] = "c2"
	var nilp *A
	m[nilp] = "nil *A"
	m[(*B)(nil)] = "nil *B"
	e1, e2 := &empty{}, &empty{}
	m[e1] = "e1"
	m[e2] = "e2"
	m[1] = "one"
	m["k"] = "k"
	var keys []string
	for k, v := range m {
		keys = append(keys, fmt.Sprintf("%T=%s", k, v))
	}
	sort.Strings(keys)
	fmt.Println(len(m), keys, m[c], m[c2], m[nilp], m[e1])
	clear(m)
	m[b] = "after clear"
	fmt.Println(len(m), m[a] == "", m[b])

	var sm sync.Map
	types := []reflect.Type{reflect.TypeOf(A{}), reflect.TypeOf(B{}), reflect.TypeOf(""), reflect.TypeOf(a)}
	for i, t := range types {
		sm.Store(t, i)
	}
	for _, t := range types {
		x, ok := sm.Load(t)
		fmt.Print(t, "=", x, ok, " ")
	}
	_, ok = sm.Load(reflect.TypeOf(0))
	fmt.Println(ok)
}
