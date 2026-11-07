// Calls of function variables and fields that hold method values and
// method expressions of concrete types: they block only if those methods
// do. A method value of an interface type may hold any implementation.
package main

import "fmt"

type counter struct{ n int }

func (c *counter) add(d int) { c.n += d }
func (c counter) get() int   { return c.n }

var ch = make(chan int, 1)

type waiter struct{}

func (waiter) wait(d int) { ch <- d; <-ch }

// hooks holds callbacks, as an options struct does.
type hooks struct {
	onMove func(int)
	read   func() int
}

// move calls hooks that hold methods that never block.
func move(h hooks, d int) int {
	h.onMove(d)
	return h.read()
}

var step func(*counter, int) = (*counter).add

// bump calls a method expression, which never blocks.
func bump(c *counter) { step(c, 2) }

// slow calls a field that holds a method value that blocks.
type slow struct{ run func(int) }

func (s slow) call(d int) { s.run(d) }

type mover interface{ wait(int) }

// viaIface calls a method value of an interface type.
func viaIface(m mover) {
	f := m.wait
	f(1)
}

func main() {
	c := &counter{}
	h := hooks{onMove: c.add, read: c.get}
	fmt.Println(move(h, 3), move(h, 4))
	bump(c)
	slow{run: waiter{}.wait}.call(5)
	viaIface(waiter{})
	fmt.Println(c.n)
}
