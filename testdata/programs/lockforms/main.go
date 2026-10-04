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

var exitedVia sync.Mutex

// exitGoroutine ends the calling goroutine.
func exitGoroutine() { runtime.Goexit() }

// lockAndExitVia is lockAndExit with the Goexit in a helper.
func lockAndExitVia(stop bool) {
	exitedVia.Lock()
	if stop {
		exitGoroutine()
	}
	exitedVia.Unlock()
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

var initLocked, valueLocked, goArgLocked, exprLocked sync.Mutex

// lockInInit locks initLocked in an if initializer and holds it across a
// receive.
func lockInInit(ch chan int) {
	if initLocked.Lock(); len(ch) == 0 {
		<-ch
		initLocked.Unlock()
	}
}

// lockByValue locks valueLocked through a method value.
func lockByValue(ch chan int) {
	lock := valueLocked.Lock
	lock()
	<-ch
	valueLocked.Unlock()
}

// goArg holds goArgLocked while a go statement's argument is received.
func goArg(ch chan int) {
	goArgLocked.Lock()
	go func(int) {}(<-ch)
	goArgLocked.Unlock()
}

// holdExpr holds exprLocked across a receive.
func holdExpr(ch chan int) {
	exprLocked.Lock()
	<-ch
	exprLocked.Unlock()
}

// contend runs hold, which locks a mutex and waits on ch, then a goroutine
// running lockUnlock, which locks the same mutex while it is held (and
// unlocks it).
func contend(hold func(chan int), lockUnlock func(), name string, ch chan int) {
	done := make(chan bool)
	go func() {
		hold(ch)
		done <- true
	}()
	go func() {
		lockUnlock()
		done <- true
	}()
	ch <- 1
	<-done
	<-done
	os.Stdout.WriteString(name + " ok\n")
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
		exited.Unlock()
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

	go func() {
		defer func() { done <- true }()
		lockAndExitVia(true)
	}()
	<-done
	go func() {
		exitedVia.Lock() // waits for the Unlock below
		exitedVia.Unlock()
		done <- true
	}()
	go func() {
		<-ch
		exitedVia.Unlock()
		done <- true
	}()
	ch <- 1
	<-done
	<-done
	os.Stdout.WriteString("locked after Goexit in a helper\n")

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
	contend(lockInInit, func() { initLocked.Lock(); initLocked.Unlock() }, "lock in initializer", ch)
	contend(lockByValue, func() { valueLocked.Lock(); valueLocked.Unlock() }, "lock by method value", ch)
	lockInit := initLocked.Lock
	contend(lockInInit, func() { lockInit(); initLocked.Unlock() }, "waiting method value", ch)
	contend(goArg, func() { goArgLocked.Lock(); goArgLocked.Unlock() }, "go statement argument", ch)
	lockExpr := (*sync.Mutex).Lock
	contend(holdExpr, func() { lockExpr(&exprLocked); exprLocked.Unlock() }, "waiting method expression value", ch)
	os.Stdout.WriteString("done\n")
}
