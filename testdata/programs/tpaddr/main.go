// Taking the address of variables of a type parameter's type, instantiated
// with scalar and aggregate types.
package main

import "fmt"

type point struct{ X, Y int }

func addr[T any](x T) *T { return &x }

func swapVia[T any](a, b T) (T, T) {
	pa, pb := &a, &b
	*pa, *pb = *pb, *pa
	return a, b
}

func assignAfterAddr[T any](x, y T) (T, T) {
	p := &x
	x = y // *p sees the new value
	return *p, x
}

func setThrough[T any](x T, v T) T {
	p := &x
	*p = v
	return x
}

func perIteration[T any](vals []T) []*T {
	var ps []*T
	for i := 0; i < len(vals); i++ {
		v := vals[i]
		ps = append(ps, &v)
	}
	return ps
}

func main() {
	fmt.Println(*addr(3), *addr("s"), *addr(point{1, 2}), *addr([2]int{3, 4}))
	fmt.Println(swapVia(1, 2))
	fmt.Println(swapVia(point{1, 2}, point{3, 4}))
	fmt.Println(assignAfterAddr(1, 2))
	fmt.Println(assignAfterAddr(point{1, 2}, point{5, 6}))
	fmt.Println(assignAfterAddr([2]string{"a", "b"}, [2]string{"c", "d"}))
	fmt.Println(setThrough(point{}, point{7, 8}), setThrough(1.5, 2.5))
	for _, p := range perIteration([]point{{1, 1}, {2, 2}}) {
		fmt.Print(*p, " ")
	}
	for _, p := range perIteration([]int{1, 2, 3}) {
		fmt.Print(*p, " ")
	}
	fmt.Println()
	p := addr(point{1, 2})
	p.X = 10
	fmt.Println(*p)
}
