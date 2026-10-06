//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of internal/strconv's atoi.go: Atoi's slow path is
// ParseInt(s, 10, 0) written out for base 10 and 64 bits, so that a program
// calling Atoi does not keep ParseInt and ParseUint with their other bases,
// bit sizes and underscores.
package strconv

// Atoi is equivalent to ParseInt(s, 10, 0), converted to type int.
func Atoi(s string) (int, error) {
	sLen := len(s)
	if intSize == 32 && (0 < sLen && sLen < 10) ||
		intSize == 64 && (0 < sLen && sLen < 19) {
		// Fast path for small integers that fit int type.
		s0 := s
		if s[0] == '-' || s[0] == '+' {
			s = s[1:]
			if len(s) < 1 {
				return 0, ErrSyntax
			}
		}

		n := 0
		for _, ch := range []byte(s) {
			ch -= '0'
			if ch > 9 {
				return 0, ErrSyntax
			}
			n = n*10 + int(ch)
		}
		if s0[0] == '-' {
			n = -n
		}
		return n, nil
	}

	// Slow path for invalid, big, or underscored integers.
	i64, err := parseInt10(s)
	return int(i64), err
}

// parseInt10 is ParseInt(s, 10, 64): ParseUint's loop for base 10, which
// stops at the first byte that is not a digit or at the first overflow,
// then ParseInt's range check.
func parseInt10(s string) (int64, error) {
	if s == "" {
		return 0, ErrSyntax
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		s = s[1:]
		neg = true
	}
	if s == "" {
		return 0, ErrSyntax
	}
	const maxUint64 = 1<<64 - 1
	var un uint64
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, ErrSyntax
		}
		if un >= maxUint64/10+1 {
			un = maxUint64 // n*10 overflows
			break
		}
		n1 := un*10 + uint64(c-'0')
		if n1 < un*10 {
			un = maxUint64 // n+d overflows
			break
		}
		un = n1
	}
	const cutoff = 1 << 63
	if !neg && un >= cutoff {
		return cutoff - 1, ErrRange
	}
	if neg && un > cutoff {
		return -cutoff, ErrRange
	}
	n := int64(un)
	if neg {
		n = -n
	}
	return n, nil
}
