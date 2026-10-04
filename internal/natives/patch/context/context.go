//go:build goesm

// goesm's patch of package context: cancelCtx.Err waits on the done
// channel, which is closed by then, so that its error is never seen before
// the channel is closed on another thread. Goroutines never run in parallel
// under goesm and cancel stores the error and closes the channel without
// yielding in between, so the wait is dropped: a receive makes a function
// async, and Context.Err is called everywhere.
package context

func (c *cancelCtx) Err() error {
	if err := c.err.Load(); err != nil {
		return err.(error)
	}
	return nil
}
