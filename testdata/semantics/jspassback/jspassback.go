// Package jspassback is called from JavaScript (test/js/poc.test.mjs),
// which passes values Go gave it back to Go. No exported function takes a
// function, so these are the only ways in.
package jspassback

import (
	"io"
	"time"
)

type slowReader struct{ s string }

// Read waits.
func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(time.Millisecond)
	return copy(p, r.s), io.EOF
}

// NewSlow returns a reader whose Read waits.
func NewSlow(s string) io.Reader { return &slowReader{s} }

// ReadAll reads r, which only JavaScript passes: no Go code calls it.
func ReadAll(r io.Reader) string {
	b := make([]byte, 16)
	n, _ := r.Read(b)
	return string(b[:n])
}

var later = func() string {
	time.Sleep(time.Millisecond)
	return "later"
}

// Later returns a function that waits, which Go stores nowhere else.
func Later() func() string { return later }

// Box is a handle whose method takes a function.
type Box struct{ calls int }

// NewBox returns a Box.
func NewBox() *Box { return &Box{} }

// Call calls f, which JavaScript may pass back from Later.
func (b *Box) Call(f func() string) string {
	b.calls++
	return f() + "!"
}
