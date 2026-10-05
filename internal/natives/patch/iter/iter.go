//go:build goesm

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
// generator yield, instead of switching goroutines.
package iter

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

// Pull steps seq's generator if it has one, and otherwise runs it on a
// coroutine as Go's Pull, which it keeps as pullCoro.
//
//goesm:original pullCoro
func Pull[V any](seq Seq[V]) (next func() (V, bool), stop func()) {
	if g := seqGen(seq); g != nil {
		return pullGen[V](g)
	}
	return pullCoro(seq)
}

// Pull2 is Pull for a Seq2.
//
//goesm:original pull2Coro
func Pull2[K, V any](seq Seq2[K, V]) (next func() (K, V, bool), stop func()) {
	if g := seqGen(seq); g != nil {
		return pull2Gen[K, V](g)
	}
	return pull2Coro(seq)
}
