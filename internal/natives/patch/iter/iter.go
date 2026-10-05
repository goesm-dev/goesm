//go:build goesm

// goesm's patch of iter's iter.go: Pull's coroutines are goroutines that the
// runtime switches between by resolving Promises (natives.ts: newcoro and
// coroswitch are internal/natives overrides). The Go bodies below are what
// the blocking analysis sees, and say the same with channels: each side
// blocks until the other switches back, so only one runs at a time, as with
// the gc runtime's coroutines.
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
