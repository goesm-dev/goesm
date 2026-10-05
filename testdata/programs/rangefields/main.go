// Command rangefields ranges over structs whose fields the loop body only
// reads, which goesm loads into locals instead of copying each element, next
// to loops that use the element otherwise.
package main

import "fmt"

type counter int

func (c *counter) inc() { *c++ }

func (c counter) String() string { return fmt.Sprintf("#%d", int(c)) }

type inner struct{ a, b int }

type item struct {
	name  string
	n     int
	c     counter
	in    inner
	fn    func() int
	ptr   *inner
	inner // embedded: in.a and the promoted a
}

func show(it item) string { return fmt.Sprintf("%s=%d", it.name, it.n) }

func main() {
	items := []item{
		{name: "a", n: 1, fn: func() int { return 10 }, ptr: &inner{1, 2}, inner: inner{3, 4}},
		{name: "b", n: 2, fn: func() int { return 20 }, ptr: &inner{5, 6}, inner: inner{7, 8}},
		{name: "c", n: 3, fn: func() int { return 30 }, ptr: &inner{9, 10}, inner: inner{11, 12}},
	}

	// The element changes after the iteration started: the variable keeps
	// the values from the start.
	for i, it := range items {
		items[i].n *= 100
		items[(i+1)%len(items)].name += "!"
		fmt.Println(i, it.name, it.n, items[i].n)
	}

	// Closures capture each iteration's fields.
	var fs []func() string
	for _, it := range items {
		fs = append(fs, func() string { return it.name })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()

	// Calls through fields, a pointer field written through, a value method.
	sum := 0
	for _, it := range items {
		sum += it.fn() + it.ptr.a
		it.ptr.b++
		fmt.Print(it.c.String(), " ")
	}
	fmt.Println(sum, items[0].ptr.b)

	// A pointer method on a field changes the copy only.
	for _, it := range items {
		it.c.inc()
		fmt.Print(it.c, " ")
	}
	fmt.Println(items[0].c)

	// Assigned fields, aggregate fields, promoted fields, whole uses.
	for _, it := range items {
		it.n++
		fmt.Print(it.n, " ")
	}
	for _, it := range items {
		fmt.Print(it.in.a, it.a, " ")
	}
	for _, it := range items {
		fmt.Print(show(it), " ")
	}
	fmt.Println()

	// Arrays (ranged over as a copy) and maps.
	arr := [2]inner{{1, 2}, {3, 4}}
	for i, x := range arr {
		arr[1-i].a = 50
		fmt.Print(x.a, x.b, " ")
	}
	m := map[string]inner{"k": {7, 8}}
	for k, x := range m {
		m[k] = inner{0, 0}
		fmt.Print(k, x.a+x.b, " ")
	}
	fmt.Println(arr)

	// Shadowing and a field with the variable's name.
	type pair struct{ it, x int }
	for _, it := range []pair{{1, 2}} {
		x := it.x
		{
			it := it.it + x
			fmt.Println(it)
		}
	}
}
