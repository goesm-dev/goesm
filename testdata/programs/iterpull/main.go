package main

import (
	"fmt"
	"iter"
	"maps"
	"runtime"
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

	// runtime.Goexit in the iterator ends the goroutine that called next,
	// after its deferred calls.
	done := make(chan bool)
	go func() {
		defer close(done)
		defer fmt.Println("goexit deferred")
		gn, _ := iter.Pull(func(yield func(int) bool) {
			yield(7)
			runtime.Goexit()
		})
		fmt.Println(gn())
		gn()
		fmt.Println("not reached")
	}()
	<-done

	// A consumer and an iterator that block on channels in between.
	ch := make(chan int)
	go func() {
		for i := range 3 {
			ch <- i * i
		}
		close(ch)
	}()
	cn, cs := iter.Pull(func(yield func(int) bool) {
		for v := range ch {
			if !yield(v) {
				return
			}
		}
	})
	for {
		v, ok := cn()
		if !ok {
			break
		}
		fmt.Println("from channel", v)
	}
	cs()
	generators()
	fmt.Println("end")
}

type point struct{ x, y int }

func say(i int) int {
	fmt.Println("say", i)
	return i
}

// generators covers sequences that Pull steps as JS generators.
func generators() {
	// A sequence that ignores yield's false keeps running after stop,
	// without yielding.
	n1, s1 := iter.Pull(func(yield func(int) bool) {
		for i := range 4 {
			fmt.Println("stubborn", i, yield(say(i)))
		}
	})
	fmt.Println(n1())
	s1()
	fmt.Println(n1())

	// Values are copied when yielded.
	n2, s2 := iter.Pull(func(yield func(point) bool) {
		p := point{1, 2}
		yield(p)
		p.x = 10
		yield(p)
	})
	a, _ := n2()
	b, _ := n2()
	fmt.Println(a, b)
	fmt.Println(n2())
	s2()

	// Interface values, nil among them.
	n3, s3 := iter.Pull(func(yield func(error) bool) {
		yield(nil)
		yield(fmt.Errorf("e%d", 1))
	})
	fmt.Println(n3())
	fmt.Println(n3())
	fmt.Println(n3())
	s3()

	// Seq2, with labeled loops and a switch.
	n4, s4 := iter.Pull2(func(yield func(string, int) bool) {
	outer:
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				switch {
				case j > i:
					continue outer
				case i == 2 && j == 1:
					break outer
				}
				if !yield(fmt.Sprint(i), j) {
					return
				}
			}
		}
	})
	for {
		k, v, ok := n4()
		if !ok {
			break
		}
		fmt.Println("pair", k, v)
	}
	s4()

	// A panic the sequence recovers itself, and deferred calls on stop.
	n5, s5 := iter.Pull(func(yield func(int) bool) {
		defer fmt.Println("n5 done")
		func() {
			defer func() { fmt.Println("n5 recovered", recover()) }()
			var m map[string]int
			m["x"] = 1
		}()
		yield(5)
		yield(6)
	})
	fmt.Println(n5())
	s5()
	fmt.Println(n5())

	// yield passed on, so not a generator.
	n6, s6 := iter.Pull(func(yield func(int) bool) {
		slices.Values([]int{8, 9})(yield)
	})
	fmt.Println(n6())
	fmt.Println(n6())
	fmt.Println(n6())
	s6()

	// Two generators of one literal, interleaved.
	g := func(base int) iter.Seq[int] {
		return func(yield func(int) bool) {
			for i := base; i < base+3; i++ {
				if !yield(i) {
					return
				}
			}
		}
	}
	x, sx := iter.Pull(g(0))
	y, sy := iter.Pull(g(100))
	for range 4 {
		v, ok := x()
		w, ok2 := y()
		fmt.Println(v, ok, w, ok2)
	}
	sx()
	sy()
}
