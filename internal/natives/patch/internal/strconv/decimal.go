//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of internal/strconv's decimal.go: goesm's uint is a JS
// number, exact only below 2^53, so the multiprecision decimal shifts by at
// most 28 bits at a time, as on 32-bit platforms; 60-bit shifts would lose
// digits.
package strconv

// Maximum shift that we can do in one pass without overflow.
const maxShift = 32 - 4
