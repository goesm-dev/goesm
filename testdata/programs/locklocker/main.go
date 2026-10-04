package main

import (
	"os"
	"sync"
)

var mu sync.Mutex
var l sync.Locker = &mu

func main() {
	started, release := make(chan bool), make(chan bool)
	go func() {
		mu.Lock()
		started <- true
		<-release // holds mu while blocked
		mu.Unlock()
	}()
	<-started
	go func() { release <- true }()
	l.Lock() // via sync.Locker: must wait for the goroutine
	os.Stdout.WriteString("locked via Locker\n")
	l.Unlock()
}
