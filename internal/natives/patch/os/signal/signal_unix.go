//go:build goesm

// goesm's patch of package os/signal: the gc runtime queues the signals the
// process receives for signal_recv (runtime/sigqueue.go). goesm listens for
// them on the host instead (process.on under Node.js and Bun; a browser
// sends none), and delivers them through a channel.
package signal

// hostSignal starts (on) or stops listening for the host signal sig.
// Listening calls deliver(sig) in a new goroutine for every signal received,
// and, as with a Go program that handles a signal, keeps it from
// terminating the process.
func hostSignal(sig uint32, on bool, deliver func(uint32))

var (
	received = make(chan uint32, numSig)
	ignored  [numSig]bool
)

func deliverSignal(sig uint32) {
	select {
	case received <- sig:
	default: // the gc runtime drops a signal that is already pending too
	}
}

func signal_enable(sig uint32) {
	if sig < numSig {
		ignored[sig] = false
	}
	hostSignal(sig, true, deliverSignal)
}

func signal_disable(sig uint32) {
	hostSignal(sig, false, nil)
}

func signal_ignore(sig uint32) {
	if sig < numSig {
		ignored[sig] = true
	}
	hostSignal(sig, true, func(uint32) {})
}

func signal_ignored(sig uint32) bool {
	return sig < numSig && ignored[sig]
}

func signal_recv() uint32 {
	return <-received
}
