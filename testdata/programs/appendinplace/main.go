// Appends to local slice variables, which goesm makes in place while no
// other variable holds the variable's slice header: every way the header
// can be copied (assignment, call, return, conversion to an interface,
// storing it, sending it, a method value, a receive into the variable)
// must leave the copies as Go leaves them.
package main

import (
	"fmt"
	"strings"
)

var kept [][]int

func keep(s []int) int { kept = append(kept, s); return len(s) }

func build(n int) []int {
	var x []int
	for i := 0; i < n; i++ {
		x = append(x, i)
	}
	return x
}

type ints []int

func (s ints) save() { kept = append(kept, s) }

type holder struct{ s []int }

// writer's fields are appended to through pointers.
type writer struct {
	b    []byte
	n    []int
	tags []string
}

func (w *writer) byte(c byte)    { w.b = append(w.b, c) }
func (w *writer) str(s string)   { w.b = append(w.b, s...) }
func (w *writer) pair(a, b byte) { w.b = append(w.b, a, b) }
func (w *writer) num(i int)      { w.n = append(w.n, i, i*2, len(w.n)) }
func (w *writer) bytes() []byte  { return w.b }
func (w *writer) reset()         { w.b = w.b[:0] }
func (w *writer) tag(t string)   { w.tags = append(w.tags, t) }
func (w *writer) self()          { w.n = append(w.n, w.n...) }
func (w *writer) keepN() int     { return keep(w.n) }
func (w *writer) later() func() int {
	return func() int { w.tags = append(w.tags, "late"); return len(w.tags) }
}

func fields() {
	w := &writer{b: make([]byte, 0, 64)}
	w.byte('a')
	w.str("bc")
	w.pair('d', 'e')
	got := w.bytes() // the caller holds the header
	w.byte('f')
	fmt.Println(string(got), string(w.bytes()))

	c := *w // a copy of the struct
	pc := &c
	w.byte('g')
	pc.byte('X') // the same array, as in Go
	fmt.Println(string(w.bytes()), string(pc.bytes()))

	var i any = *w
	w.byte('h')
	iw := i.(writer)
	fmt.Println(string((&iw).bytes()), string(w.bytes()))

	w.reset()
	w.str("zz")
	fmt.Println(string(w.bytes()), string(got))

	for k := 0; k < 3; k++ {
		w.num(k)
	}
	before := w.keepN()
	w.num(9)
	w.self()
	fmt.Println(w.n, before, kept[len(kept)-1])

	w2 := w // another pointer to the same struct
	w2.tag("x")
	w.tag("y")
	f := w.later()
	f()
	w.tag("z")
	n := f()
	fmt.Println(w.tags, w2.tags, n)

	ws := []writer{{}, {}}
	for j := range ws {
		p := &ws[j]
		p.byte(byte('0' + j))
		p.byte(byte('0' + j))
	}
	cp := ws[0]
	(&ws[0]).byte('!')
	fmt.Println(string((&ws[0]).bytes()), string((&ws[1]).bytes()), string((&cp).bytes()))
}

func tokens(s string) []string {
	var out []string
	var cur []rune
	for _, r := range s {
		if r == ' ' {
			if len(cur) > 0 {
				out = append(out, string(cur))
			}
			cur = cur[:0]
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func main() {
	x := build(5)
	y := x // y holds x's header
	x = append(x, 100)
	x = append(x, 101)
	fmt.Println(x, y, len(y), cap(y) >= len(y))

	// A call that keeps the header, in an element of the append itself.
	a := make([]int, 0, 10)
	a = append(a, 1)
	a = append(a, keep(a))
	a = append(a, 3)
	fmt.Println(a, kept[0], len(kept[0]))

	// Slicing shares the array, as in Go.
	b := make([]int, 0, 8)
	b = append(b, 1, 2, 3)
	c := b[:2]
	b = append(b, 4)
	c = append(c, 9) // overwrites b[2]
	fmt.Println(b, c)

	// Appending nothing, then something.
	d := []int{1, 2}
	d = append(d, 3)
	e := d
	d = append(d, []int{}...)
	d = append(d, 4)
	fmt.Println(d, e)

	// Appending a slice to itself.
	f := make([]int, 0, 16)
	f = append(f, 1, 2)
	f = append(f, f...)
	f = append(f, f[1:3]...)
	fmt.Println(f)

	// Bytes and strings.
	var g []byte
	for i := 0; i < 3; i++ {
		g = append(g, "ab"...)
		g = append(g, '0'+byte(i))
	}
	h := string(g)
	g = append(g, '!')
	fmt.Println(h, string(g))

	// Interfaces, fields, maps, channels, method values.
	var i []int
	i = append(i, 1)
	var any1 any = i
	i = append(i, 2)
	hd := holder{s: i}
	i = append(i, 3)
	m := map[string][]int{"k": i}
	i = append(i, 4)
	ch := make(chan []int, 1)
	ch <- i
	i = append(i, 5)
	ints(i).save()
	i = append(i, 6)
	sv := ints(i).save
	i = append(i, 7)
	sv()
	fmt.Println(i, any1, hd.s, m["k"], <-ch, kept[1], kept[2])

	// A receive and a range into the variable.
	ch <- []int{7, 8}
	j := make([]int, 0, 4)
	j = append(j, 0)
	select {
	case j = <-ch:
	}
	j = append(j, 9)
	fmt.Println(j)
	src := [][]int{{1}, {2, 3}}
	var k []int
	for _, k = range src {
		k = append(k, 0)
	}
	fmt.Println(k, src)

	// Variables of loops.
	base := make([]int, 2, 10)
	for round := 0; round < 2; round++ {
		for l := base; len(l) < 4; l = append(l, round) {
			fmt.Print(len(l), " ")
		}
		cur := base
		cur = append(cur, round)
		cur = append(cur, round)
		fmt.Println(cur, base)
	}

	// Results.
	r := build(3)
	r2 := r
	r = append(r, 7)
	fmt.Println(r, r2)

	fmt.Printf("%q\n", tokens("a bc  def g"))

	// Goroutines and defer get the header too.
	done := make(chan int)
	n := make([]int, 0, 8)
	n = append(n, 1)
	go func(s []int) { done <- len(s) }(n)
	n = append(n, 2)
	fmt.Println(<-done, n)

	var sb strings.Builder
	w := []string{}
	for _, s := range []string{"x", "y", "z"} {
		w = append(w, s)
		sb.WriteString(strings.Join(w, ","))
		sb.WriteByte(';')
	}
	fmt.Println(sb.String())

	fields()
}
