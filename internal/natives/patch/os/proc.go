//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of os's proc.go: Args is initialized by its declaration
// instead of by init, so that a bundler drops it, and os with it, from a
// program that does not use them (runtime_args is pure for the lowering).
package os

// Args hold the command-line arguments, starting with the program name.
var Args []string = runtime_args()

func init() {}
