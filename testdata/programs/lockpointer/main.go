package main

import (
	"os"
	"sync"
)

var mu sync.Mutex

func withLock(m *sync.Mutex, f func()) { m.Lock(); f(); m.Unlock() }

func main() {
	started, release := make(chan bool), make(chan bool)
	go func() {
		mu.Lock()
		started <- true
		<-release
		mu.Unlock()
	}()
	<-started
	go func() { release <- true }()
	withLock(&mu, func() { os.Stdout.WriteString("in withLock\n") })
}
