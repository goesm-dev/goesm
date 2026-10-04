// Assorted language corners: indexing and addressing through type
// parameters, type switches on a type parameter, promoted fields in
// composite literals, multi-valued arguments to builtins, assignment order
// with failing assignments, reflect map iteration, bounds errors and
// recover only in the deferred function itself.
package main

import (
	"fmt"
	"math"
	"reflect"
)

func setIdx[T interface{ []int | [3]int }](x T) int {
	x[1] = 7
	return x[1] + len(x)
}

type setter[B any] interface {
	Set(string)
	*B
}

type named struct{ s string }

func (n *named) Set(s string) { n.s = s }

type counter int

func (c *counter) Set(s string) { *c = counter(len(s)) }

func fill[T any, PT setter[T]](ss []string) []T {
	r := make([]T, len(ss))
	for i, s := range ss {
		PT(&r[i]).Set(s)
	}
	return r
}

func kind[T any](i any) string {
	switch x := i.(type) {
	case T:
		return fmt.Sprint("T ", x)
	case int:
		return "int"
	default:
		return "other"
	}
}

type pair[A, B any] struct {
	a A
	b B
}

type C struct{ c any }
type B struct {
	b string
	C
}
type A struct {
	a int
	B
}

func two(m map[string]int) (map[string]int, string) { return m, "x" }

func catch(f func()) (msg string) {
	defer func() { msg = fmt.Sprint(recover()) }()
	f()
	return
}

func callRecover() any { return recover() }

func viaHelper() { fmt.Println("helper recovered:", callRecover()) }

func direct() { fmt.Println("direct recovered:", recover()) }

func recursive(n int) {
	if n == 0 {
		recursive(1)
		fmt.Println("outer recovered:", recover())
		return
	}
	fmt.Println("inner recovered:", recover())
}

func recovers(f func()) {
	defer func() { fmt.Println("left over:", recover()) }()
	f()
}

func main() {
	recovers(func() {
		defer direct()
		defer viaHelper()
		panic("a")
	})
	recovers(func() {
		defer recursive(0)
		panic("b")
	})
	recovers(func() {
		defer func() {
			defer recover() // recovers: called by the deferred closure
		}()
		panic("c")
	})
	recovers(func() {
		defer recover() // does not recover: not called by a deferred function
		panic("d")
	})
	recovers(func() {
		defer func() { func() { fmt.Println("nested literal:", recover()) }() }()
		panic("e")
	})

	fmt.Println(setIdx([]int{1, 2, 3}), setIdx([3]int{}))
	fmt.Println(fill[named]([]string{"a", "b"}), fill[counter]([]string{"abc"}))
	fmt.Println(kind[float64](1.5), kind[float64](1), kind[any](2), kind[fmt.Stringer](3))

	type mine struct {
		a int
		b string
	}
	fmt.Printf("%+v\n", mine(pair[int, string]{1, "x"}))
	fmt.Printf("%+v %+v\n", A{a: 1, b: "foo"}, A{c: "bar"})

	m := map[string]int{"x": 1, "y": 2}
	delete(two(m))
	fmt.Println(len(m))

	var p *struct{ i int }
	m2 := map[int]int{}
	fmt.Println(catch(func() { m2[1], p.i = 1, 2 }), len(m2))

	r := reflect.ValueOf(map[float64]int{math.NaN(): 1, math.NaN(): 2})
	n := 0
	for it := r.MapRange(); it.Next(); {
		n += int(it.Value().Int())
	}
	fmt.Println(n)

	s := []int{1, 2, 3}
	i, j := -1, 5
	fmt.Println(catch(func() { _ = s[i] }))
	fmt.Println(catch(func() { _ = s[1:j] }))
	fmt.Println(catch(func() { _ = s[i+3 : 1 : j] }))
	fmt.Println(catch(func() { _ = make([]int, j-10) }))
	println(-(1 << 63), uint64(1<<64-1))
}
