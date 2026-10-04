package main

import (
	"os"
	"sync"
)

// A struct embedding a Mutex, held by a blocked goroutine and locked through
// sync.Locker: the promoted Lock is the Mutex's own.
type counter struct {
	sync.Mutex
	n int
}

func main() {
	c := &counter{}
	var l sync.Locker = c
	started, release := make(chan bool), make(chan bool)
	go func() {
		c.Lock()
		started <- true
		<-release // holds the mutex while blocked
		c.n++
		c.Unlock()
	}()
	<-started
	go func() { release <- true }()
	l.Lock()
	c.n++
	l.Unlock()
	os.Stdout.WriteString("n=" + string(rune('0'+c.n)) + "\n")
}
