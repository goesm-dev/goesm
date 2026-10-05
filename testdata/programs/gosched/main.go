// Command gosched polls flags with runtime.Gosched: Gosched yields to the
// other goroutines and to the host's timers, so the loops see what
// they change.
package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

func main() {
	var byTimer atomic.Bool
	time.AfterFunc(5*time.Millisecond, func() { byTimer.Store(true) })
	for !byTimer.Load() {
		runtime.Gosched()
	}
	fmt.Println("timer fired")

	var bySleeper atomic.Bool
	go func() {
		time.Sleep(time.Millisecond)
		bySleeper.Store(true)
	}()
	for !bySleeper.Load() {
		runtime.Gosched()
	}
	fmt.Println("sleeping goroutine woke")
}
