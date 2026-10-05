// Calls of interface methods and function values of which only some block:
// a call of one that does not block takes its result as it is, without
// awaiting it.
package main

import (
	"errors"
	"fmt"
	"strings"
)

type Sink interface{ Put(v int) }

type sum struct{ n int }

func (s *sum) Put(v int) { s.n += v }

type pipe struct{ c chan int }

func (s pipe) Put(v int) { s.c <- v }

type failing struct{}

func (failing) Put(v int) {
	if v == 3 {
		panic(errors.New("put 3"))
	}
}

func fill(s Sink, n int) {
	for i := 0; i < n; i++ {
		s.Put(i)
	}
}

func apply(fs []func(int) int, v int) []int {
	var out []int
	for _, f := range fs {
		out = append(out, f(v))
	}
	return out
}

func main() {
	s := &sum{}
	fill(s, 10)
	fmt.Println("sum", s.n)

	c := make(chan int)
	done := make(chan int)
	go func() {
		t := 0
		for v := range c {
			t += v
		}
		done <- t
	}()
	fill(pipe{c}, 10)
	close(c)
	fmt.Println("pipe", <-done)

	func() {
		defer func() { fmt.Println("recovered", recover()) }()
		fill(failing{}, 10)
	}()

	in := make(chan int, 1)
	fs := []func(int) int{
		func(v int) int { return v + 1 },
		func(v int) int { in <- v; return <-in * 2 },
		func(v int) int { return len(strings.Repeat("x", v)) },
	}
	fmt.Println(apply(fs, 5))
}
