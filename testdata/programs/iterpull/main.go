package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
)

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer fmt.Println("count done")
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

func zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	return func(yield func(A, B) bool) {
		na, sa := iter.Pull(a)
		defer sa()
		nb, sb := iter.Pull(b)
		defer sb()
		for {
			x, ok1 := na()
			y, ok2 := nb()
			if !ok1 || !ok2 || !yield(x, y) {
				return
			}
		}
	}
}

func main() {
	next, stop := iter.Pull(count(3))
	for {
		v, ok := next()
		fmt.Println(v, ok)
		if !ok {
			break
		}
	}
	stop()
	fmt.Println(next())

	next, stop = iter.Pull(count(10))
	fmt.Println(next())
	fmt.Println(next())
	stop()
	stop()
	fmt.Println(next())

	for a, b := range zip(slices.Values([]string{"a", "b", "c"}), count(5)) {
		fmt.Println(a, b)
	}

	n2, s2 := iter.Pull2(maps.All(map[string]int{"x": 1}))
	fmt.Println(n2())
	fmt.Println(n2())
	s2()

	pn, ps := iter.Pull(func(yield func(int) bool) {
		yield(1)
		panic("boom")
	})
	fmt.Println(pn())
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		pn()
	}()
	fmt.Println(pn())
	ps()

	// stop before next.
	_, s3 := iter.Pull(count(2))
	s3()
	fmt.Println("end")
}
