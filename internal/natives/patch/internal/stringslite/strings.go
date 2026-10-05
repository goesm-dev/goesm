//go:build goesm

// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of internal/stringslite's strings.go: Clone returns s. A
// Go string is an immutable engine string, which holds no larger string's
// memory that a copy would let go of.
package stringslite

func Clone(s string) string {
	return s
}
