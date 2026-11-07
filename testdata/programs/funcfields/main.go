// Calls through function fields and function parameters that only ever
// hold known functions, and calls in branches a constant condition never
// runs: they block only if one of those functions does, although other
// function values of the same signature block.
package main

import (
	"fmt"
	"strings"
	"sync"
)

// cache calls its New, like sync.Pool's Get.
type cache struct {
	New  func() []byte
	free [][]byte
}

func (c *cache) get() []byte {
	if n := len(c.free); n > 0 {
		b := c.free[n-1]
		c.free = c.free[:n-1]
		return b[:0]
	}
	return c.New()
}

func (c *cache) put(b []byte) { c.free = append(c.free, b) }

var small = cache{New: func() []byte { return make([]byte, 0, 8) }}

var large = &cache{New: newLarge}

func newLarge() []byte { return make([]byte, 0, 64) }

// render calls get through both caches, which never block.
func render(words []string) string {
	b := small.get()
	for _, w := range words {
		b = append(b, w...)
	}
	s := string(b)
	small.put(b)
	l := large.get()
	l = append(l, strings.ToUpper(s)...)
	large.put(l)
	return string(l)
}

// each calls f, which its callers pass only functions that never block.
func each(xs []int, f func(int)) {
	for _, x := range xs {
		f(x)
	}
}

// twice passes its own f on to each.
func twice(xs []int, f func(int)) {
	each(xs, f)
	each(xs, f)
}

var sum int

func addSum(x int) { sum += x }

func count(xs []int) int {
	n := 0
	each(xs, func(int) { n++ })
	twice(xs, addSum)
	return n
}

const debug = false

var trace = make(chan string, 1)

// note waits only in a branch that never runs.
func note(s string) string {
	if debug {
		trace <- s
		<-trace
	}
	return "note " + s
}

var pool = sync.Pool{New: func() any { return new(strings.Builder) }}

// join uses a sync.Pool, whose New never blocks.
func join(xs []string) string {
	b := pool.Get().(*strings.Builder)
	b.Reset()
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(x)
	}
	s := b.String()
	pool.Put(b)
	return s
}

// shape gets its Area from a value of an unnamed struct type, without a
// store to the field: area blocks, since that function does.
type shape struct{ Area func() int }

func area(s shape) int { return s.Area() }

var ch = make(chan int, 1)

func main() {
	// A func() []byte, a func(int) and a func() any that block.
	makers := []func() []byte{func() []byte { ch <- 1; return make([]byte, <-ch) }}
	visits := []func(int){func(x int) { ch <- x; sum += <-ch }}
	anys := []func() any{func() any { ch <- 2; return <-ch }}
	fmt.Println(render([]string{"a", "b"}), render([]string{"c"}))
	fmt.Println(count([]int{1, 2, 3}), sum)
	fmt.Println(note("x"), join([]string{"p", "q"}), join(nil))
	fmt.Println(len(makers[0]()), anys[0]())
	visits[0](4)
	fmt.Println(sum)
	var sq shape = struct{ Area func() int }{func() int { ch <- 9; return <-ch }}
	fmt.Println(area(sq))
}
