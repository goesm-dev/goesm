package main

import (
	"os"
	"sync"
)

// Two waiters on a Cond; the first one woken blocks while holding the lock.
func main() {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	ready := false
	release := make(chan bool)
	done := make(chan bool)
	for i := 0; i < 2; i++ {
		go func(i int) {
			mu.Lock()
			for !ready {
				cond.Wait()
			}
			if i == 0 {
				<-release // blocks while holding mu
			}
			mu.Unlock()
			done <- true
		}(i)
	}
	go func() {
		mu.Lock()
		ready = true
		cond.Broadcast()
		mu.Unlock()
		release <- true
	}()
	<-done
	<-done
	os.Stdout.WriteString("ok\n")
}
