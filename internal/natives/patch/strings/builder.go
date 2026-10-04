//go:build goesm

// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strings' builder.go: a Builder holds a string that grows
// by concatenation, which engines represent as a rope (no copy per write),
// instead of a []byte (a Uint8Array) that String converts to a string with
// a copy. Cap reports the capacity the []byte would have, grown as append
// grows slices in goesm (runtime/src/slice.ts).
package strings

import "unicode/utf8"

// A Builder is used to efficiently build a string using [Builder.Write] methods.
// It minimizes memory copying. The zero value is ready to use.
// Do not copy a non-zero Builder.
type Builder struct {
	addr *Builder // of receiver, to detect copies by value

	s   string
	c   int    // Cap
	buf []byte // unused: the original methods are still type-checked
}

func (b *Builder) copyCheck() {
	if b.addr == nil {
		b.addr = b
	} else if b.addr != b {
		panic("strings: illegal use of non-zero Builder copied by value")
	}
}

// String returns the accumulated string.
func (b *Builder) String() string { return b.s }

// Len returns the number of accumulated bytes; b.Len() == len(b.String()).
func (b *Builder) Len() int { return len(b.s) }

// Cap returns the capacity of the builder's underlying byte slice. It is the
// total space allocated for the string being built and includes any bytes
// already written.
func (b *Builder) Cap() int { return b.c }

// Reset resets the [Builder] to be empty.
func (b *Builder) Reset() {
	b.addr = nil
	b.s = ""
	b.c = 0
}

// grow copies the buffer to a new, larger buffer so that there are at least n
// bytes of capacity beyond len(b.buf).
func (b *Builder) grow(n int) {
	b.c = 2*b.c + n
}

// Grow grows b's capacity, if necessary, to guarantee space for
// another n bytes. After Grow(n), at least n bytes can be written to b
// without another allocation. If n is negative, Grow panics.
func (b *Builder) Grow(n int) {
	b.copyCheck()
	if n < 0 {
		panic("strings.Builder.Grow: negative count")
	}
	if b.c-len(b.s) < n {
		b.grow(n)
	}
}

// appended records that the string grew to n bytes.
func (b *Builder) appended(n int) {
	if n <= b.c {
		return
	}
	c := b.c
	doubled := c + c
	switch {
	case n > doubled:
		c = n
	case c < 256:
		c = doubled
	default:
		for c < n {
			c += (c + 3*256) >> 2
		}
	}
	b.c = c
}

// Write appends the contents of p to b's buffer.
// Write always returns len(p), nil.
func (b *Builder) Write(p []byte) (int, error) {
	b.copyCheck()
	b.s += string(p)
	b.appended(len(b.s))
	return len(p), nil
}

// WriteByte appends the byte c to b's buffer.
// The returned error is always nil.
func (b *Builder) WriteByte(c byte) error {
	b.copyCheck()
	b.s += byteString(c)
	b.appended(len(b.s))
	return nil
}

// WriteRune appends the UTF-8 encoding of Unicode code point r to b's buffer.
// It returns the length of r and a nil error.
func (b *Builder) WriteRune(r rune) (int, error) {
	b.copyCheck()
	n := len(b.s)
	if uint32(r) < utf8.RuneSelf {
		b.s += byteString(byte(r))
	} else {
		b.s += string(r)
	}
	b.appended(len(b.s))
	return len(b.s) - n, nil
}

// WriteString appends the contents of s to b's buffer.
// It returns the length of s and a nil error.
func (b *Builder) WriteString(s string) (int, error) {
	b.copyCheck()
	b.s += s
	b.appended(len(b.s))
	return len(s), nil
}

// byteString is the string of the one byte c.
func byteString(c byte) string
