// Command appendforms exercises the specialized forms of append (one value,
// a slice, a string) and comparisons under basic-typed type parameters.
package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

type point struct{ x, y int }

func eq[T cmp.Ordered](a, b T) bool { return a == b }

func index[T comparable](s []T, v T) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func main() {
	// One value, growing from nil and within capacity.
	var a []int
	for i := range 10 {
		a = append(a, i*i)
	}
	fmt.Println(a, len(a), cap(a) >= 10)
	b := a[:3]
	b = append(b, -1) // overwrites a[3]
	fmt.Println(a[3], b)

	// A slice, aliasing the destination's backing array.
	c := []int{1, 2, 3, 4}
	c = append(c[:1], c[:3]...)
	fmt.Println(c)
	d := make([]int, 2, 10)
	d = append(d[:1], d[:2]...)
	fmt.Println(d)
	e := append([]int(nil), a...)
	e[0] = 99
	fmt.Println(a[0], e[0], len(append(e, []int{}...)))

	// Strings into a []byte, within capacity and growing.
	buf := make([]byte, 0, 4)
	buf = append(buf, "ab"...)
	buf = append(buf, "cdefgh"...)
	buf = append(buf, ""...)
	buf = append(buf, 'z')
	fmt.Println(string(buf), len(buf))
	big := make([]byte, 0)
	for range 2000 {
		big = append(big, "héllo, "...)
	}
	s := string(big)
	fmt.Println(len(s), s[len(s)-9:])

	// Aggregates still copy.
	ps := []point{{1, 2}}
	ps = append(ps, ps[0])
	ps[1].x = 7
	qs := append([]point(nil), ps...)
	qs[0].y = 9
	fmt.Println(ps, qs)
	arrs := [][2]int{{1, 2}}
	arrs = append(arrs, arrs[0])
	arrs[1][0] = 5
	fmt.Println(arrs)

	// Basic type parameters compare with ==; NaN != NaN.
	nan := math.NaN()
	fmt.Println(eq(1, 1), eq("a", "b"), eq(nan, nan), eq(0.0, math.Copysign(0, -1)))
	fmt.Println(index([]float64{1, nan, 2}, nan), index([]string{"x", "y"}, "y"))
	fmt.Println(slices.Index([]point{{1, 2}, {3, 4}}, point{3, 4}))
	fmt.Println(slices.Max([]int{3, 9, 2}), slices.Sorted(slices.Values([]string{"b", "a"})))
}
