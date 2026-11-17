// Functions that block through calls whose callees only the whole program
// tells: interface methods, function values and function parameters.
// goesm emit-ts -blocking explains each down to the operation that waits
// (test/blocking_test.go checks the explanations).
package main

import (
	"fmt"
	"time"
)

type source interface{ next() int }

type queue struct{ ch chan int }

func (q queue) next() int { return <-q.ch }

type fixed int

func (f fixed) next() int { return int(f) }

// read calls an interface method: queue's blocks.
func read(s source) int { return s.next() }

var handlers = map[string]func() string{}

func nap() string {
	time.Sleep(time.Millisecond)
	return "nap"
}

// dispatch calls a function value.
func dispatch(name string) string { return handlers[name]() }

// each calls its function parameter.
func each(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

// total passes each a literal that blocks.
func total(s source) int {
	sum := 0
	each(2, func(int) { sum += read(s) })
	return sum
}

// even and odd call each other before they read.
func even(n int, s source) int {
	if n > 0 {
		return odd(n-1, s)
	}
	return read(s)
}

func odd(n int, s source) int {
	if n > 0 {
		return even(n-1, s)
	}
	return -read(s)
}

// label blocks nowhere.
func label(n int) string { return fmt.Sprint("n=", n) }

func main() {
	handlers["nap"] = nap
	q := queue{make(chan int, 4)}
	for i := 1; i <= 4; i++ {
		q.ch <- i
	}
	fmt.Println(read(fixed(7)), dispatch("nap"), total(q), even(2, q), odd(2, q), label(1))
}
