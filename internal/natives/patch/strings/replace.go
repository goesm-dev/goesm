//go:build goesm

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of strings' replace.go: a Replacer replaces with the
// engine's String.prototype.replace and a RegExp of its old strings in
// argument order (replacerOf and replaceAll in runtime/src/natives.ts)
// instead of Go's trie and byte tables. An alternation tries its
// alternatives in order at each position, which is the priority Go's
// generic replacer gives the old strings, and the engine's global
// replace takes a match that is empty, as an empty old string's, at most
// once at each position, as Go's does; a Go string's code units are its
// bytes. The Go algorithms, about 8 KB minified, are left out.
package strings

import (
	"io"
	"unsafe"
)

func (b *Replacer) build() replacer {
	return &jsReplacer{replacerOf(b.oldnew)}
}

// jsReplacer holds the engine's RegExp of the old strings and the map from
// each to its new string.
type jsReplacer struct{ r unsafe.Pointer }

func (r *jsReplacer) Replace(s string) string { return replaceAll(r.r, s) }

func (r *jsReplacer) WriteString(w io.Writer, s string) (n int, err error) {
	return io.WriteString(w, replaceAll(r.r, s))
}

func replacerOf(oldnew []string) unsafe.Pointer
func replaceAll(r unsafe.Pointer, s string) string
