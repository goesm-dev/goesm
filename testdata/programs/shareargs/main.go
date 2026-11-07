// Command shareargs checks that struct and array arguments passed without a
// copy (internal/lower/sharearg.go) still behave as copies: the callee sees
// the value at the time of the call however the caller's value changes, and
// the caller never sees what the callee does.
package main

import "fmt"

type Point struct{ X, Y float64 }

type Box struct {
	Min, Max Point
	Tags     [2]string
}

var global = Point{1, 2}

func area(b Box) float64 { return (b.Max.X - b.Min.X) * (b.Max.Y - b.Min.Y) }

func center(b Box) Point { return Point{(b.Min.X + b.Max.X) / 2, (b.Min.Y + b.Max.Y) / 2} }

// through passes its parameter on to another reader.
func through(b Box) float64 { return area(b) + center(b).X }

// writeThenRead writes through q, which may point at the caller's p.
func writeThenRead(p Point, q *Point) float64 {
	q.X = 100
	return p.X
}

// writeGlobalThenRead writes the global the caller may pass.
func writeGlobalThenRead(p Point) float64 {
	global.X = 50
	return p.X
}

// callThenRead calls f, which may change the caller's p.
func callThenRead(p Point, f func()) float64 {
	f()
	return p.X
}

// keep returns its parameter; the caller's later changes must not show.
func keep(p Point) Point { return p }

func keepArray(a [2]string) [2]string { return a }

// store keeps its parameter in a struct the caller owns.
func store(dst *holder, p Point) { dst.p = p }

// stash keeps its parameter in a slice.
func stash(p Point) []Point { return []Point{p} }

// modify changes its own copy.
func modify(p Point) float64 {
	p.X++
	return p.X
}

func sum(ps [3]Point) float64 {
	t := 0.0
	for _, p := range ps {
		t += p.X + p.Y
	}
	return t
}

type holder struct{ p Point }

func (h *holder) read(p Point) float64 {
	h.p.X = -1
	return p.X
}

// fresh returns a local, which goesm returns without a copy.
func fresh(x float64) Point {
	p := Point{x, x}
	p.Y++
	return p
}

// twice returns one local twice: the two results must be two values.
func twice() (Point, Point) {
	p := Point{1, 2}
	return p, p
}

var kept *Point

// escaped returns a local a pointer to which it keeps.
func escaped() Point {
	p := Point{3, 4}
	kept = &p
	return p
}

// closed returns a local a closure it keeps writes.
var later func()

func closed() Point {
	p := Point{5, 6}
	later = func() { p.X = -5 }
	return p
}

type counter struct{ n [2]int }

func (c *counter) inc() { c.n[0]++ }

// method returns a local whose pointer method value it keeps.
var incLater func()

func method() counter {
	c := counter{}
	incLater = c.inc
	return c
}

// named returns its result variables.
func named(x float64) (p, q Point, err error) {
	p = Point{x, 0}
	q = p
	q.Y = 1
	return p, p, nil
}

func namedBare() (p Point) {
	p.X = 4
	return
}

func namedDefer() (p Point) {
	defer func() { p.Y = 8 }()
	p.X = 7
	return p
}

func main() {
	n1, n2, _ := named(3)
	n1.X = -3
	fmt.Println(n1, n2)
	nb1, nb2 := namedBare(), namedBare()
	nb1.Y = 1
	fmt.Println(nb1, nb2, namedDefer())

	f1, f2 := fresh(1), fresh(2)
	f1.X = 10
	fmt.Println(f1, f2)
	t1, t2 := twice()
	t1.X = 100
	fmt.Println(t1, t2)
	e := escaped()
	kept.X = 30
	fmt.Println(e, *kept)
	cl := closed()
	later()
	fmt.Println(cl)
	mc := method()
	incLater()
	fmt.Println(mc)
	lit := func() Box {
		b := Box{Max: Point{1, 1}}
		return b
	}
	l1, l2 := lit(), lit()
	l1.Max.X = 9
	fmt.Println(l1.Max, l2.Max)

	b := Box{Point{0, 0}, Point{4, 2}, [2]string{"a", "b"}}
	fmt.Println(area(b), center(b), through(b))

	p := Point{1, 1}
	fmt.Println(writeThenRead(p, &p), p)

	g := global
	fmt.Println(writeGlobalThenRead(global), global, g)

	c := Point{3, 3}
	fmt.Println(callThenRead(c, func() { c.X = 30 }), c)

	k := keep(p)
	p.X = 7
	fmt.Println(k, p)

	arr := [2]string{"x", "y"}
	ka := keepArray(arr)
	arr[0] = "z"
	fmt.Println(ka, arr)

	q := Point{2, 2}
	kq := keep(q)
	var hq holder
	store(&hq, q)
	sq := stash(q)
	q.X = 8
	fmt.Println(kq, hq.p, sq, q)

	m := Point{5, 5}
	fmt.Println(modify(m), m)

	ps := [3]Point{{1, 2}, {3, 4}, {5, 6}}
	fmt.Println(sum(ps), sum([3]Point{ps[2], ps[1], ps[0]}))

	h := &holder{Point{9, 9}}
	fmt.Println(h.read(h.p), h.p)

	boxes := []Box{b, {Point{1, 1}, Point{2, 3}, [2]string{}}}
	total := 0.0
	for i := range boxes {
		total += area(boxes[i]) + center(boxes[i]).Y
	}
	fmt.Println(total)

	defer func(p Point) { fmt.Println("deferred", p) }(m)
	m.X = 0
}
