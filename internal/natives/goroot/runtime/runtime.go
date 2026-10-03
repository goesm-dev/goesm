//go:build goesm

// Package runtime is goesm's replacement for the gc runtime package: the
// exported API that the rest of the standard library and user code may use.
// Memory management, scheduling and type metadata live in @goesm/runtime;
// functions without a body here are implemented there (runtime/src/natives.ts).
package runtime

const (
	GOOS     = "js"
	GOARCH   = "wasm"
	Compiler = "goesm"
)

// MemProfileRate has no effect under goesm.
var MemProfileRate int = 0

// Error identifies a run time error.
type Error interface {
	error
	RuntimeError()
}

// A TypeAssertionError explains a failed type assertion.
type TypeAssertionError struct {
	_interface    string
	concrete      string
	asserted      string
	missingMethod string
}

func (*TypeAssertionError) RuntimeError() {}

func (e *TypeAssertionError) Error() string {
	inter := "interface"
	if e._interface != "" {
		inter = e._interface
	}
	if e.concrete == "" {
		return "interface conversion: " + inter + " is nil, not " + e.asserted
	}
	if e.missingMethod == "" {
		return "interface conversion: " + inter + " is " + e.concrete + ", not " + e.asserted
	}
	return "interface conversion: " + e.concrete + " is not " + e.asserted + ": missing method " + e.missingMethod
}

// A PanicNilError happens when code calls panic(nil).
type PanicNilError struct {
	_ [0]*PanicNilError
}

func (*PanicNilError) Error() string {
	return "panic called with nil argument (use runtime.PanicNilError)"
}
func (*PanicNilError) RuntimeError() {}

// Version returns the Go tree's version string.
func Version() string { return "goesm" }

// GOROOT returns the empty string: there is no Go tree at run time.
func GOROOT() string { return "" }

// Goexit terminates the goroutine that calls it.
func Goexit()

// Gosched yields the processor, allowing other goroutines to run: it waits
// for a goroutine started behind the ones already runnable.
func Gosched() {
	ch := make(chan struct{})
	go close(ch)
	<-ch
}

// GOMAXPROCS reports 1: JavaScript runs goroutines on one thread.
func GOMAXPROCS(n int) int { return 1 }

// NumCPU reports 1.
func NumCPU() int { return 1 }

// NumGoroutine returns the number of goroutines that currently exist.
func NumGoroutine() int

// NumCgoCall returns 0.
func NumCgoCall() int64 { return 0 }

// KeepAlive marks its argument as currently reachable.
func KeepAlive(x any) {}

// SetFinalizer is a no-op under goesm: the JavaScript collector runs no Go
// finalizers.
func SetFinalizer(obj any, finalizer any) {}

// Cleanup is a handle to a cleanup call for a specific object.
type Cleanup struct{}

// AddCleanup is a no-op under goesm (see SetFinalizer).
func AddCleanup[T, S any](ptr *T, cleanup func(S), arg S) Cleanup { return Cleanup{} }

// Stop cancels the cleanup call.
func (c Cleanup) Stop() {}

// GC runs a garbage collection: a no-op under goesm.
func GC() {}

// LockOSThread and UnlockOSThread have no effect: there is one thread.
func LockOSThread()   {}
func UnlockOSThread() {}

// Breakpoint executes a breakpoint trap.
func Breakpoint() { panic("runtime.Breakpoint") }

// Caller reports no caller information under goesm.
func Caller(skip int) (pc uintptr, file string, line int, ok bool) { return 0, "", 0, false }

// Callers reports no program counters under goesm.
func Callers(skip int, pc []uintptr) int { return 0 }

// Stack formats no stack trace under goesm.
func Stack(buf []byte, all bool) int { return 0 }

// Frames may be used to get function/file/line information for a slice of
// PC values returned by Callers.
type Frames struct {
	callers []uintptr
}

// Frame is the information returned by Frames for each call frame.
type Frame struct {
	PC        uintptr
	Func      *Func
	Function  string
	File      string
	Line      int
	startLine int
	Entry     uintptr
}

// CallersFrames takes a slice of PCs returned by Callers.
func CallersFrames(callers []uintptr) *Frames { return &Frames{callers: callers} }

// Next returns a Frame representing the next call frame.
func (ci *Frames) Next() (frame Frame, more bool) { return Frame{}, false }

// A Func represents a Go function in the running binary.
type Func struct {
	opaque struct{}
}

// FuncForPC returns nil under goesm.
func FuncForPC(pc uintptr) *Func { return nil }

func (f *Func) Name() string                                { return "" }
func (f *Func) Entry() uintptr                              { return 0 }
func (f *Func) FileLine(pc uintptr) (file string, line int) { return "", 0 }

// MemStats records statistics about the memory allocator; goesm reports zeros.
type MemStats struct {
	Alloc, TotalAlloc, Sys, Lookups, Mallocs, Frees                    uint64
	HeapAlloc, HeapSys, HeapIdle, HeapInuse, HeapReleased, HeapObjects uint64
	StackInuse, StackSys, MSpanInuse, MSpanSys, MCacheInuse, MCacheSys uint64
	BuckHashSys, GCSys, OtherSys, NextGC, LastGC, PauseTotalNs         uint64
	PauseNs                                                            [256]uint64
	PauseEnd                                                           [256]uint64
	NumGC, NumForcedGC                                                 uint32
	GCCPUFraction                                                      float64
	EnableGC, DebugGC                                                  bool
	BySize                                                             [61]struct {
		Size           uint32
		Mallocs, Frees uint64
	}
}

// ReadMemStats populates m with zeros.
func ReadMemStats(m *MemStats) { *m = MemStats{} }

func SetMutexProfileFraction(rate int) int { return 0 }
func SetBlockProfileRate(rate int)         {}
func SetCPUProfileRate(hz int)             {}
