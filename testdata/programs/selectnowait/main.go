// Command selectnowait checks that the channel operations of a select with
// a default case do not make a function async: draining, trying to send,
// checking for a closed channel, and time.Timer's Stop and Reset and
// time.AfterFunc, whose timers do the same.
package main

import (
	"fmt"
	"time"
)

func drain(c chan int) int {
	n := 0
	for {
		select {
		case <-c:
			n++
		default:
			return n
		}
	}
}

func trySend(c chan int, v int) bool {
	select {
	case c <- v:
		return true
	default:
		return false
	}
}

func isClosed(c chan struct{}) bool {
	select {
	case _, ok := <-c:
		return !ok
	default:
		return false
	}
}

func stopTimer() bool {
	t := time.NewTimer(time.Hour)
	return t.Stop()
}

func resetTimer() bool {
	t := time.NewTimer(time.Hour)
	t.Reset(2 * time.Hour)
	return t.Stop()
}

func cancelAfter() bool {
	t := time.AfterFunc(time.Hour, func() { fmt.Println("never") })
	return t.Stop()
}

func main() {
	c := make(chan int, 4)
	fmt.Println(trySend(c, 1), trySend(c, 2))
	fmt.Println(drain(c))
	done := make(chan struct{})
	fmt.Println(isClosed(done))
	close(done)
	fmt.Println(isClosed(done))
	fmt.Println(stopTimer(), resetTimer(), cancelAfter())
	// A receive that waits, so the program has async functions.
	r := make(chan string)
	go func() { r <- "received" }()
	fmt.Println(<-r)
}
