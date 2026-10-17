// Calls of function variables that only ever hold known functions: they
// block only if one of those does, although other function values of the
// same signature block.
package main

import "fmt"

var (
	total  int
	update func()
	step   = add
)

func add(i, delta int) { total += i * delta }

// handle calls update, which never blocks.
func handle(i, delta int) {
	step(i, delta)
	update()
}

// relay calls a variable whose address is taken: it may hold any func().
func relay() {
	f := func() {}
	p := &f
	*p = waitOne
	f()
}

var ch = make(chan int, 1)

func waitOne() { ch <- 1; <-ch }

func main() {
	var log []string
	update = func() { log = append(log, fmt.Sprint(total)) }
	// A func(int, int) and a func() that block.
	blocking := []func(int, int){func(i, d int) { ch <- i; total += <-ch * d }}
	waits := []func(){waitOne}
	handle(2, 3)
	handle(1, -1)
	blocking[0](4, 1)
	waits[0]()
	relay()
	fmt.Println(total, log)
}
