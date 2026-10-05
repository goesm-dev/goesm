//go:build goesm

// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of fmt's errors.go: Errorf formats with fastSprintf (see
// print.go) when it can, as Sprintf does, and wraps the one %w operand
// the way errorf does after a pp formatted the message.
package fmt

import (
	"errors"
	"internal/stringslite"
	"slices"
)

// errorf formats and returns an error value, or nil if no formatting is required.
func errorf(format string, a ...any) error {
	if len(a) == 0 && stringslite.IndexByte(format, '%') == -1 {
		return nil
	}
	if s, w, ok := fastSprintf(format, a, true); ok {
		if w < 0 {
			return errors.New(s)
		}
		e := &wrapError{msg: s}
		e.err, _ = a[w].(error)
		return e
	}
	p := newPrinter()
	p.wrapErrs = true
	p.doPrintf(format, a)
	s := string(p.buf)
	var err error
	switch len(p.wrappedErrs) {
	case 0:
		err = errors.New(s)
	case 1:
		w := &wrapError{msg: s}
		w.err, _ = a[p.wrappedErrs[0]].(error)
		err = w
	default:
		if p.reordered {
			slices.Sort(p.wrappedErrs)
		}
		var errs []error
		for i, argNum := range p.wrappedErrs {
			if i > 0 && p.wrappedErrs[i-1] == argNum {
				continue
			}
			if e, ok := a[argNum].(error); ok {
				errs = append(errs, e)
			}
		}
		err = &wrapErrors{s, errs}
	}
	p.free()
	return err
}
