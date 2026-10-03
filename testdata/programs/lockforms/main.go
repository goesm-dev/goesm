// Command lockforms locks mutexes held across a channel receive through
// method expressions and through a type parameter constrained by
// sync.Locker; the second Lock has to wait.
package main

import (
	"os"
	"sync"
)

var mu sync.Mutex

func hold[L sync.Locker](l L, ch chan int, kind string) {
	l.Lock()
	v := <-ch
	os.Stdout.WriteString(kind + " got " + string(rune('0'+v)) + "\n")
	l.Unlock()
}

func main() {
	(*sync.Mutex).Lock(&mu)
	(*sync.Mutex).Unlock(&mu)

	ch := make(chan int)
	done := make(chan bool)
	var m sync.Mutex
	go func() { hold(&m, ch, "generic"); done <- true }()
	go func() { hold(&m, ch, "generic"); done <- true }()
	ch <- 1
	ch <- 2
	<-done
	<-done

	var rw sync.RWMutex
	for _, kind := range []string{"expr", "expr"} {
		go func() {
			(*sync.RWMutex).Lock(&rw)
			v := <-ch
			os.Stdout.WriteString(kind + " got " + string(rune('0'+v)) + "\n")
			rw.Unlock()
			done <- true
		}()
	}
	ch <- 1
	ch <- 2
	<-done
	<-done

	var l sync.Locker = &m
	for _, kind := range []string{"locker", "locker"} {
		go func() {
			sync.Locker.Lock(l)
			v := <-ch
			os.Stdout.WriteString(kind + " got " + string(rune('0'+v)) + "\n")
			l.Unlock()
			done <- true
		}()
	}
	ch <- 1
	ch <- 2
	<-done
	<-done
	os.Stdout.WriteString("done\n")
}
