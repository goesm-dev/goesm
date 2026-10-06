// Loops over slices whose array, offset and length the lowering loads once
// (hoistSliceHeaders), range keys used as the loop counter, short variable
// declarations without temporaries and op-assignments of plain references.
package main

import "fmt"

type body struct{ x, v float64 }

func step(bs []body) {
	for i := range bs {
		b := &bs[i]
		for j := i + 1; j < len(bs); j++ {
			b.v += bs[j].x
			bs[j].v -= b.x
		}
	}
	for i := range bs {
		bs[i].x += bs[i].v
	}
}

func sum(s []int) int {
	t := 0
	for i := 0; i < len(s); i++ {
		t += s[i]
	}
	for _, v := range s {
		t += v
	}
	return t
}

func sub(s []int) int {
	t := 0
	for i := range s[1:] {
		t += s[i+1]
	}
	return t
}

func closures(s []int) []func() int {
	var fs []func() int
	for i := range s {
		fs = append(fs, func() int { return s[i] * i })
	}
	return fs
}

func keyAssigned(s []int) []int {
	var out []int
	for i := range s {
		out = append(out, i)
		i += 10
		out = append(out, i)
	}
	return out
}

func swap(a, b int) (int, int) {
	a, b = b, a
	x, y := b, a
	return x, y
}

var calls int

func next() int {
	calls++
	return calls
}

func opAssign() {
	b := &body{x: 1}
	b.x += 2
	s0 := []float64{1, 2, 3}
	s0[next()] += b.x
	s0[next()]++
	fmt.Println(b.x, s0, calls)
	n := 3
	n *= n + 1
	s := []int{1, 2, 3}
	i := 0
	s[i] += 10
	i++
	s[i] -= i
	fmt.Println(n, s)
}

func shadow() {
	x, y := 1, 2
	{
		x, y := y, x
		fmt.Println(x, y)
	}
	a, b := x+y, x*y
	fmt.Println(a, b)
}

func main() {
	bs := []body{{x: 1}, {x: 2}, {x: 3}}
	for k := 0; k < 3; k++ {
		step(bs)
	}
	fmt.Println(bs)
	step(nil)
	step(bs[:0])
	fmt.Println(sum([]int{1, 2, 3}), sum(nil), sub([]int{4, 5, 6}))
	for _, f := range closures([]int{7, 8, 9}) {
		fmt.Print(f(), " ")
	}
	fmt.Println()
	fmt.Println(keyAssigned([]int{1, 2}))
	fmt.Println(swap(1, 2))
	opAssign()
	shadow()
	defer func() { fmt.Println("recovered:", recover()) }()
	var empty []int
	for i := 0; i < len(empty)+1; i++ {
		fmt.Println(empty[i])
	}
}
