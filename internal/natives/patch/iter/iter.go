//go:build goesm

// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of iter's iter.go: Pull's coroutines are goroutines that the
// runtime switches between by resolving Promises (natives.ts: newcoro and
// coroswitch are internal/natives overrides). The Go bodies below are what
// the blocking analysis sees, and say the same with channels: each side
// blocks until the other switches back, so only one runs at a time, as with
// the gc runtime's coroutines.
//
// A sequence that is a function literal calling yield only directly, and
// blocking on nothing else, also has a JS generator (see the lowering's
// seqGenerator): Pull and Pull2 then step it synchronously, each yield a
// generator yield, instead of switching goroutines. Pull and Pull2 are Go's,
// with that fast path first. They are copies rather than calls of the
// originals (//goesm:original) because a generic call from Pull would
// instantiate them with Pull's own type parameters, and their blocking yield
// function would then match the calls of every func(T) bool value.
package iter

import (
	"internal/race"
	"runtime"
	"unsafe"
)

type coro struct {
	inside bool          // the coroutine is running
	resume chan struct{} // switches into the coroutine
	back   chan struct{} // switches out of it
}

func newcoro(f func(*coro)) *coro {
	c := &coro{resume: make(chan struct{}), back: make(chan struct{})}
	go func() {
		<-c.resume
		// Also on runtime.Goexit: control returns to the last switcher when
		// the coroutine ends.
		defer func() {
			c.inside = false
			c.back <- struct{}{}
		}()
		f(c)
	}()
	return c
}

func coroswitch(c *coro) {
	if c.inside {
		c.inside = false
		c.back <- struct{}{}
		<-c.resume
		return
	}
	c.inside = true
	c.resume <- struct{}{}
	<-c.back
}

// A genSeq is the running generator of a sequence (natives.ts).
type genSeq struct{ _ int }

// seqGen starts the generator of seq, or returns nil if it has none.
func seqGen(seq any) *genSeq

// genNext resumes g, which runs until the sequence's next yield, and
// reports whether it yielded: false once the sequence has returned.
func genNext(g *genSeq) bool

// genValue returns the value of the last yield, boxed.
func genValue(g *genSeq) any

// genKey returns the key of a Seq2's last yield, boxed.
func genKey(g *genSeq) any

// genStop makes yield return false and resumes g until the sequence
// returns.
func genStop(g *genSeq)

func pullGen[V any](g *genSeq) (next func() (V, bool), stop func()) {
	done := false
	next = func() (v V, ok bool) {
		if done {
			return
		}
		if !genNext(g) {
			done = true
			return
		}
		if x := genValue(g); x != nil {
			v = x.(V)
		}
		return v, true
	}
	stop = func() {
		if !done {
			done = true
			genStop(g)
		}
	}
	return next, stop
}

func pull2Gen[K, V any](g *genSeq) (next func() (K, V, bool), stop func()) {
	done := false
	next = func() (k K, v V, ok bool) {
		if done {
			return
		}
		if !genNext(g) {
			done = true
			return
		}
		if x := genKey(g); x != nil {
			k = x.(K)
		}
		if y := genValue(g); y != nil {
			v = y.(V)
		}
		return k, v, true
	}
	stop = func() {
		if !done {
			done = true
			genStop(g)
		}
	}
	return next, stop
}

func Pull[V any](seq Seq[V]) (next func() (V, bool), stop func()) {
	if g := seqGen(seq); g != nil {
		return pullGen[V](g)
	}
	var pull struct {
		v          V
		ok         bool
		done       bool
		yieldNext  bool
		seqDone    bool // to detect Goexit
		racer      int
		panicValue any
	}
	c := newcoro(func(c *coro) {
		race.Acquire(unsafe.Pointer(&pull.racer))
		if pull.done {
			race.Release(unsafe.Pointer(&pull.racer))
			return
		}
		yield := func(v1 V) bool {
			if pull.done {
				return false
			}
			if !pull.yieldNext {
				panic("iter.Pull: yield called again before next")
			}
			pull.yieldNext = false
			pull.v, pull.ok = v1, true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))
			return !pull.done
		}
		// Recover and propagate panics from seq.
		defer func() {
			if p := recover(); p != nil {
				pull.panicValue = p
			} else if !pull.seqDone {
				pull.panicValue = goexitPanicValue
			}
			pull.done = true // Invalidate iterator
			race.Release(unsafe.Pointer(&pull.racer))
		}()
		seq(yield)
		var v0 V
		pull.v, pull.ok = v0, false
		pull.seqDone = true
	})
	next = func() (v1 V, ok1 bool) {
		race.Write(unsafe.Pointer(&pull.racer)) // detect races

		if pull.done {
			return
		}
		if pull.yieldNext {
			panic("iter.Pull: next called again before yield")
		}
		pull.yieldNext = true
		race.Release(unsafe.Pointer(&pull.racer))
		coroswitch(c)
		race.Acquire(unsafe.Pointer(&pull.racer))

		// Propagate panics and goexits from seq.
		if pull.panicValue != nil {
			if pull.panicValue == goexitPanicValue {
				// Propagate runtime.Goexit from seq.
				runtime.Goexit()
			} else {
				panic(pull.panicValue)
			}
		}
		return pull.v, pull.ok
	}
	stop = func() {
		race.Write(unsafe.Pointer(&pull.racer)) // detect races

		if !pull.done {
			pull.done = true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))

			// Propagate panics and goexits from seq.
			if pull.panicValue != nil {
				if pull.panicValue == goexitPanicValue {
					// Propagate runtime.Goexit from seq.
					runtime.Goexit()
				} else {
					panic(pull.panicValue)
				}
			}
		}
	}
	return next, stop
}

func Pull2[K, V any](seq Seq2[K, V]) (next func() (K, V, bool), stop func()) {
	if g := seqGen(seq); g != nil {
		return pull2Gen[K, V](g)
	}
	var pull struct {
		k          K
		v          V
		ok         bool
		done       bool
		yieldNext  bool
		seqDone    bool
		racer      int
		panicValue any
	}
	c := newcoro(func(c *coro) {
		race.Acquire(unsafe.Pointer(&pull.racer))
		if pull.done {
			race.Release(unsafe.Pointer(&pull.racer))
			return
		}
		yield := func(k1 K, v1 V) bool {
			if pull.done {
				return false
			}
			if !pull.yieldNext {
				panic("iter.Pull2: yield called again before next")
			}
			pull.yieldNext = false
			pull.k, pull.v, pull.ok = k1, v1, true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))
			return !pull.done
		}
		// Recover and propagate panics from seq.
		defer func() {
			if p := recover(); p != nil {
				pull.panicValue = p
			} else if !pull.seqDone {
				pull.panicValue = goexitPanicValue
			}
			pull.done = true // Invalidate iterator.
			race.Release(unsafe.Pointer(&pull.racer))
		}()
		seq(yield)
		var k0 K
		var v0 V
		pull.k, pull.v, pull.ok = k0, v0, false
		pull.seqDone = true
	})
	next = func() (k1 K, v1 V, ok1 bool) {
		race.Write(unsafe.Pointer(&pull.racer)) // detect races

		if pull.done {
			return
		}
		if pull.yieldNext {
			panic("iter.Pull2: next called again before yield")
		}
		pull.yieldNext = true
		race.Release(unsafe.Pointer(&pull.racer))
		coroswitch(c)
		race.Acquire(unsafe.Pointer(&pull.racer))

		// Propagate panics and goexits from seq.
		if pull.panicValue != nil {
			if pull.panicValue == goexitPanicValue {
				// Propagate runtime.Goexit from seq.
				runtime.Goexit()
			} else {
				panic(pull.panicValue)
			}
		}
		return pull.k, pull.v, pull.ok
	}
	stop = func() {
		race.Write(unsafe.Pointer(&pull.racer)) // detect races

		if !pull.done {
			pull.done = true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))

			// Propagate panics and goexits from seq.
			if pull.panicValue != nil {
				if pull.panicValue == goexitPanicValue {
					// Propagate runtime.Goexit from seq.
					runtime.Goexit()
				} else {
					panic(pull.panicValue)
				}
			}
		}
	}
	return next, stop
}
