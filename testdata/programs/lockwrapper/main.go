package main

import (
	"os"
	"sync"
)

type C struct {
	mu sync.Mutex
	n  int
}

func (c *C) lock()   { c.mu.Lock() }
func (c *C) unlock() { c.mu.Unlock() }

func main() {
	c := &C{}
	started, release := make(chan bool), make(chan bool)
	go func() {
		c.lock()
		started <- true
		<-release
		c.n++
		c.unlock()
	}()
	<-started
	go func() { release <- true }()
	c.lock()
	c.n++
	c.unlock()
	os.Stdout.WriteString("n=" + string(rune('0'+c.n)) + "\n")
}
