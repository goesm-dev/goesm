//go:build goesm

package signal

// signalWaitUntilIdle has nothing to wait for: a signal reaches the program
// as a goroutine, which Stop's caller cannot observe before it runs.
func signalWaitUntilIdle() {}
