//go:build goesm

// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of slices' sort.go: Sort of integers and strings is the
// engine's built-in sort (sortBuiltin in runtime/src/natives.ts). Equal
// integers or strings cannot be told apart, so any correct sort gives Go's
// result; floating-point elements (NaN, -0) keep Go's pdqsort.
package slices

import (
	"cmp"
	"math/bits"
)

// Sort sorts a slice of any ordered type in ascending order.
// When sorting floating-point numbers, NaNs are ordered before other values.
func Sort[S ~[]E, E cmp.Ordered](x S) {
	if sortBuiltin([]E(x)) {
		return
	}
	n := len(x)
	pdqsortOrdered(x, 0, n, bits.Len(uint(n)))
}

// sortBuiltin sorts x, a slice of an ordered type, with the engine's sort
// if its elements are integers or strings, and reports whether it did.
func sortBuiltin(x any) bool
