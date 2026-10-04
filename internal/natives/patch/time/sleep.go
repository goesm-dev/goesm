//go:build goesm

// goesm's patch of package time (internal/natives.Patch): timers run on the
// host's timers. The gc runtime implements them in Go (runtime/time.go,
// reached through linkname declarations); here a timer is armed with
// armTimer (setTimeout in runtime/src/natives.ts) and its function runs in
// a new goroutine when it fires.
//
// Timer channels are buffered, as with GODEBUG=asynctimerchan=1; Stop and
// Reset drain them, so as with Go 1.23's synchronous timer channels no
// stale value is received after they return (len(t.C) can still be 1).
package time

import "unsafe"

// The Timer type represents a single event. When the Timer expires, the
// current time will be sent on C, unless the Timer was created by AfterFunc.
// A Timer must be created with NewTimer or AfterFunc.
type Timer struct {
	C         <-chan Time
	initTimer bool
	host      *hostTimer
}

// A Ticker holds a channel that delivers “ticks” of a clock at intervals.
type Ticker struct {
	C          <-chan Time // The channel on which the ticks are delivered.
	initTicker bool
	host       *hostTimer
}

// hostTimer is the state of a timer: what it calls and the host timer it
// is armed with.
type hostTimer struct {
	f      func(any, uintptr, int64)
	arg    any
	c      chan Time // drained by Stop and Reset; nil for AfterFunc
	period int64
	id     int     // the armed host timer, or 0
	seq    uintptr // tells a stale firing from the current one
}

// armTimer calls fire(delta) in a new goroutine at the monotonic time when
// (runtimeNano), delta nanoseconds late, and every period nanoseconds after
// that if period > 0, until disarmTimer(id).
func armTimer(when, period int64, fire func(delta int64)) (id int)

func disarmTimer(id int)

// Sleep pauses the current goroutine for at least the duration d.
// A negative or zero duration causes Sleep to return immediately.
func Sleep(d Duration) {
	if d <= 0 {
		return
	}
	<-NewTimer(d).C
}

func syncTimer(c chan Time) unsafe.Pointer { return nil }

func newTimer(when, period int64, f func(any, uintptr, int64), arg any, cp unsafe.Pointer) *Timer {
	t := &Timer{initTimer: true, host: newHostTimer(f, arg)}
	t.host.start(when, period)
	return t
}

func newHostTimer(f func(any, uintptr, int64), arg any) *hostTimer {
	h := &hostTimer{f: f, arg: arg}
	h.c, _ = arg.(chan Time)
	return h
}

func (h *hostTimer) start(when, period int64) {
	h.period = period
	h.seq++
	seq := h.seq
	h.id = armTimer(when, period, func(delta int64) {
		if h.seq != seq {
			return // stopped or reset since
		}
		if h.period == 0 {
			h.id = 0
		}
		h.f(h.arg, seq, delta)
	})
}

// stop disarms h and reports whether it was armed.
func (h *hostTimer) stop() bool {
	h.seq++
	if h.c != nil {
		select {
		case <-h.c:
		default:
		}
	}
	if h.id == 0 {
		return false
	}
	disarmTimer(h.id)
	h.id = 0
	return true
}

func stopTimer(t *Timer) bool { return t.host.stop() }

func resetTimer(t *Timer, when, period int64) bool {
	active := t.host.stop()
	t.host.start(when, period)
	return active
}

// NewTicker returns a new Ticker containing a channel that will send the
// current time on the channel after each tick. The period of the ticks is
// specified by the duration argument. The ticker will adjust the time
// interval or drop ticks to make up for slow receivers. The duration d must
// be greater than zero; if not, NewTicker will panic.
func NewTicker(d Duration) *Ticker {
	if d <= 0 {
		panic("non-positive interval for NewTicker")
	}
	c := make(chan Time, 1)
	t := &Ticker{C: c, initTicker: true, host: newHostTimer(sendTime, c)}
	t.host.start(when(d), int64(d))
	return t
}

// Stop turns off a ticker. After Stop, no more ticks will be sent.
func (t *Ticker) Stop() {
	if !t.initTicker {
		return
	}
	t.host.stop()
}

// Reset stops a ticker and resets its period to the specified duration.
// The next tick will arrive after the new period elapses. The duration d
// must be greater than zero; if not, Reset will panic.
func (t *Ticker) Reset(d Duration) {
	if d <= 0 {
		panic("non-positive interval for Ticker.Reset")
	}
	if !t.initTicker {
		panic("time: Reset called on uninitialized Ticker")
	}
	t.host.stop()
	t.host.start(when(d), int64(d))
}
