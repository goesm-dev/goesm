// Command elemaddr compares the addresses of slice elements, &a[i] ==
// &b[j], of slices that share an array and of slices that do not, and
// checks the bounds of both operands in order.
package main

import "fmt"

type pair struct{ a, b int }

func try(name string, f func() bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	fmt.Println(name, f())
}

func main() {
	a := make([]int, 8)
	b := a[2:6]
	c := make([]int, 8)
	var arr [4]int
	d := arr[:]
	e := arr[1:]
	fmt.Println(&a[2] == &b[0], &a[3] == &b[0], &a[0] != &c[0], &b[1] == &a[3])
	fmt.Println(&d[1] == &e[0], &d[0] == &e[0], &arr[2] == &e[1], &d[3] != &e[2])
	p := &a[5]
	fmt.Println(p == &b[3], p == &c[5])

	// regexp's machine.add: len(cap) > 0 && &t.cap[0] != &cap[0].
	cp := []int{1, 2}
	t := cp
	fmt.Println(len(cp) > 0 && &t[0] != &cp[0], len(cp) > 0 && &c[0] != &cp[0])

	// Aggregate elements: the element's own identity.
	ps := []pair{{1, 2}, {3, 4}}
	qs := ps[1:]
	fmt.Println(&ps[1] == &qs[0], &ps[0] == &qs[0])

	var nilSlice []int
	try("out of range first", func() bool { return &b[4] == &a[0] })
	try("out of range second", func() bool { return &a[0] == &b[-len(b)+4] })
	try("nil", func() bool { return &nilSlice[0] == &a[0] })
	i := 9
	try("both out of range", func() bool { return &a[i] == &c[i+1] })
}
