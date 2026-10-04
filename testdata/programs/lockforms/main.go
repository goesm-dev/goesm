// Command lockforms locks mutexes held across a channel receive through
// method expressions and through a type parameter constrained by
// sync.Locker, and a Lock whose function may return with the mutex still
// locked; the second Lock has to wait.
package main

import (
	"os"
	"runtime"
	"strconv"
	"sync"
)

var mu sync.Mutex

func hold[L sync.Locker](l L, ch chan int, kind string) {
	l.Lock()
	v := <-ch
	os.Stdout.WriteString(kind + " got " + string(rune('0'+v)) + "\n")
	l.Unlock()
}

var owned sync.Mutex

// acquire returns with owned locked when keep is set.
func acquire(keep bool) bool {
	owned.Lock()
	if keep {
		return true
	}
	owned.Unlock()
	return false
}

var exited sync.Mutex

// lockAndExit ends its goroutine with exited locked when stop is set.
func lockAndExit(stop bool) {
	exited.Lock()
	if stop {
		runtime.Goexit()
	}
	exited.Unlock()
}

var tried sync.RWMutex

// tryHold holds tried across a channel receive if TryLock succeeds.
func tryHold(ch chan int) bool {
	if tried.TryLock() {
		<-ch
		tried.Unlock()
		return true
	}
	return false
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

	locked := make(chan bool)
	go func() {
		acquire(true)
		locked <- true
		v := <-ch
		os.Stdout.WriteString("owner got " + string(rune('0'+v)) + "\n")
		owned.Unlock()
		done <- true
	}()
	<-locked
	go func() {
		owned.Lock()
		os.Stdout.WriteString("second locked\n")
		owned.Unlock()
		done <- true
	}()
	ch <- 1
	<-done
	<-done

	go func() {
		defer func() { done <- true }()
		lockAndExit(true)
	}()
	<-done
	go func() {
		exited.Lock() // waits for the Unlock below
		done <- true
	}()
	go func() {
		<-ch
		exited.Unlock()
		done <- true
	}()
	ch <- 1
	<-done
	<-done
	os.Stdout.WriteString("locked after Goexit\n")

	held := make(chan bool, 1)
	go func() {
		held <- tryHold(ch)
	}()
	for tried.TryRLock() {
		tried.RUnlock() // until the TryLock in tryHold holds it
		runtime.Gosched()
	}
	go func() {
		tried.RLock() // waits for tryHold's Unlock
		tried.RUnlock()
		done <- true
	}()
	ch <- 1
	<-done
	os.Stdout.WriteString("TryLock: " + strconv.FormatBool(<-held) + ", then RLock\n")
	os.Stdout.WriteString("done\n")
}
