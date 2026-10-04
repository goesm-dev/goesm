//go:build goesm

// goesm's patch of iter's iter.go: Pull's coroutines run on a goroutine of
// their own, and coroswitch hands control over through channels. Each side
// blocks until the other switches back, so only one runs at a time, as with
// the runtime's coroutines.
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
