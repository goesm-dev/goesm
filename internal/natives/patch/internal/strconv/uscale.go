//go:build goesm

// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of internal/strconv's uscale.go: prescale computes the
// 128-bit mantissa of 10**p (pow10 in runtime/src/natives.ts) instead of
// reading pow10tab.go's table, which would be 40 KB of BigInt literals in
// every bundle that formats floats.
package strconv

// prescale returns the scaling constants for e, p.
// lp must be log2Pow10(p).
// The caller is responsible for either avoiding e, p pairs
// that cause pre.s < 0 or pre.s >= 64, or else handling
// those cases before passing the result to uscale.
// In practice, pre.s < 0 would indicate a buggy caller
// and pre.s >= 64 can only happen for parsing and is
// picked off at those call sites.
func prescale(pre *scaler, e, p, lp int) {
	pre.pmHi, pre.pmLo = pow10(p)
	pre.s = -(e + lp + 3)
}

// pow10 returns pow10Tab[p-pow10Min].
func pow10(p int) (hi, lo uint64)
