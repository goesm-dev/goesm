// Command sparecap uses the spare capacity of slices of structs and arrays,
// whose elements goesm makes only when a slice first reaches them: through
// reslicing, append, unsafe and reflect.
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type pt struct{ X, Y int }

func main() {
	// make with a capacity, then append in place and reslice past the length.
	s := make([]pt, 1, 4)
	s = append(s, pt{1, 2})
	full := s[:cap(s)]
	fmt.Println(len(full), full)
	full[3].X = 9
	s = append(s, pt{3, 4})
	fmt.Println(s, full)

	// A pointer into the spare capacity sees what append stores there.
	t := make([]pt, 0, 3)
	p := &t[:2][1]
	t = append(t, pt{5, 6}, pt{7, 8})
	fmt.Println(*p, t)
	p.X = 70
	fmt.Println(t[1])

	// Growing leaves spare capacity too.
	var g []pt
	for i := range 5 {
		g = append(g, pt{i, -i})
	}
	g2 := g[:cap(g)]
	fmt.Println(len(g), cap(g), g2[len(g)], g2[len(g2)-1])
	g2[len(g2)-1].Y = 99
	h := g[2:3:4]
	h = append(h, pt{8, 8})
	fmt.Println(h, g[:4])

	// Arrays as elements, and a three-index slice.
	a := make([][2]int, 0, 2)
	b := append(a, [2]int{1, 2})
	fmt.Println(a[:2], b)
	c := a[0:2:2]
	c[1][0] = 5
	fmt.Println(b[:2])

	// unsafe.Slice over spare capacity, and unsafe.SliceData of an empty
	// slice with capacity.
	u := make([]pt, 1, 3)
	v := unsafe.Slice(&u[0], 3)
	v[2].X = 3
	fmt.Println(v, u[:3])
	w := make([]pt, 0, 2)
	d := unsafe.SliceData(w)
	d.Y = 4
	fmt.Println(w[:1])

	// reflect: SetLen and Slice3 past the length.
	r := make([]pt, 0, 3)
	rv := reflect.ValueOf(&r).Elem()
	rv.SetLen(2)
	rv.Index(1).Field(0).SetInt(11)
	fmt.Println(r, rv.Slice3(0, 3, 3).Index(2).Interface())

	// copy into resliced capacity, and clear.
	q := make([]pt, 0, 4)
	n := copy(q[:3], []pt{{1, 1}, {2, 2}})
	fmt.Println(n, q[:4])
	clear(q[:4])
	fmt.Println(q[:4])

	// Generic code.
	fmt.Println(fill[pt](3), fill[[1]string](2), fill[int](2))
}

func fill[T any](n int) []T {
	s := make([]T, 0, n)
	var z T
	s = append(s, z)
	return s[:n]
}
