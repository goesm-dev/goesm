//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of fmt's print.go: Sprintf formats the common cases (the
// verbs %v %d %s %t %x %X %f %F %e %E %g %G of strings, booleans, integers
// and floats of the predeclared types, %v and %s of errors and Stringers,
// with the flags - and 0, a width and a precision for floats) by
// concatenating strings, which the engine does without copying, instead of
// through a pp and its []byte. Anything else, down to a missing or extra
// argument, takes Go's own code, whose output fastSprintf would have to
// reproduce. Before fastSprintf, jsSprintf tries the most common of these
// cases with the format parsed once. Errorf (errors.go) uses fastSprintf
// too, with %w.
package fmt

import (
	"reflect"
	"strconv"
	"unicode/utf8"
)

// Sprintf formats according to a format specifier and returns the resulting string.
func Sprintf(format string, a ...any) string {
	if s, ok := jsSprintf(format, a); ok {
		return s
	}
	if s, _, ok := fastSprintf(format, a, false); ok {
		return s
	}
	p := newPrinter()
	p.doPrintf(format, a)
	s := string(p.buf)
	p.free()
	return s
}

// fastSprintf is Sprintf(format, a...) for the cases described above; ok
// is false for any other. With wrapErrs, as for Errorf, it also formats
// one %w of an error, and wrapped is the index of its argument (-1 if
// there is none).
func fastSprintf(format string, a []any, wrapErrs bool) (s string, wrapped int, ok bool) {
	wrapped = -1
	// The Error and String methods are called once the whole format is
	// known to be handled here, so that none is called twice when fmt's own
	// code takes over: parts holds the text with a place for each result.
	var parts []string
	var calls []methodCall
	argNum := 0
	start := 0
	end := len(format)
	for i := 0; i < end; {
		if format[i] != '%' {
			i = indexPercent(format, i)
			if i < 0 {
				break
			}
		}
		s += format[start:i]
		i++
		if i < end && format[i] == '%' {
			s += "%"
			i++
			start = i
			continue
		}
		minus, zero := false, false
	flags:
		for ; i < end; i++ {
			switch format[i] {
			case '-':
				minus, zero = true, false
			case '0':
				zero = !minus
			default:
				break flags
			}
		}
		wid := -1
		if i < end && '0' <= format[i] && format[i] <= '9' {
			wid = 0
			for ; i < end && '0' <= format[i] && format[i] <= '9'; i++ {
				if wid > 1e5 {
					return "", -1, false
				}
				wid = wid*10 + int(format[i]-'0')
			}
		}
		prec := -1
		if i < end && format[i] == '.' {
			i++
			prec = 0
			for ; i < end && '0' <= format[i] && format[i] <= '9'; i++ {
				if prec > 1e5 {
					return "", -1, false
				}
				prec = prec*10 + int(format[i]-'0')
			}
		}
		if i >= end || argNum >= len(a) || format[i] >= utf8.RuneSelf {
			return "", -1, false
		}
		verb := format[i]
		i++
		start = i
		arg := a[argNum]
		argNum++
		var num string // a number, padded with zeros after its sign
		switch v := arg.(type) {
		case int:
			num = fmtInt(v, verb, prec)
		case string:
			if (verb != 's' && verb != 'v') || prec >= 0 || zero {
				return "", -1, false
			}
			s += padded(v, wid, minus)
			continue
		case float64:
			num = fastFloat(v, 64, verb, prec)
		case bool:
			if (verb != 't' && verb != 'v') || prec >= 0 || zero {
				return "", -1, false
			}
			s += padded(strconv.FormatBool(v), wid, minus)
			continue
		case uint8:
			num = fmtInt(int(v), verb, prec)
		case int32:
			num = fmtInt(int(v), verb, prec)
		case int64:
			num = fastInt64(v, verb, prec)
		case uint64:
			num = fastUint64(v, verb, prec)
		case uint:
			num = fmtUint(v, verb, prec)
		case uint32:
			num = fmtInt(int(v), verb, prec)
		case int8:
			num = fmtInt(int(v), verb, prec)
		case int16:
			num = fmtInt(int(v), verb, prec)
		case uint16:
			num = fmtInt(int(v), verb, prec)
		case uintptr:
			num = fmtUint(uint(v), verb, prec)
		case float32:
			num = fastFloat(float64(v), 32, verb, prec)
		default:
			// An error or a Stringer, as handleMethods prints them.
			if (verb != 's' && verb != 'v' && (verb != 'w' || !wrapErrs || wrapped >= 0)) || prec >= 0 || zero {
				return "", -1, false
			}
			switch arg.(type) {
			case Formatter, reflect.Value:
				// printArg prints the value a reflect.Value holds, not
				// its String.
				return "", -1, false
			case error:
			case Stringer:
				if verb == 'w' {
					return "", -1, false
				}
			default:
				return "", -1, false
			}
			if verb == 'w' {
				wrapped = argNum - 1
			}
			parts = append(parts, s, "")
			calls = append(calls, methodCall{len(parts) - 1, arg, verb, wid, minus})
			s = ""
			continue
		}
		if num == "" {
			return "", -1, false
		}
		if zero && wid > len(num) && num != "NaN" && num != "+Inf" && num != "-Inf" {
			sign := ""
			if num[0] == '-' {
				sign, num = "-", num[1:]
			}
			for n := wid - len(sign) - len(num); n > 0; n-- {
				num = "0" + num
			}
			num = sign + num
		}
		s += padded(num, wid, minus)
	}
	if argNum != len(a) {
		return "", -1, false
	}
	if calls == nil {
		return s + format[start:], wrapped, true
	}
	for _, c := range calls {
		parts[c.at] = c.text()
	}
	out := ""
	for _, p := range parts {
		out += p
	}
	return out + s + format[start:], wrapped, true
}

// A methodCall is an error or Stringer operand of fastSprintf, whose text
// goes to parts[at].
type methodCall struct {
	at    int
	arg   any
	verb  byte
	wid   int
	minus bool
}

// text returns the operand's Error or String, padded, or what catchPanic
// prints if the method panics.
func (c methodCall) text() (s string) {
	method := "Error"
	defer func() {
		if err := recover(); err != nil {
			if v := reflect.ValueOf(c.arg); v.Kind() == reflect.Pointer && v.IsNil() {
				s = nilAngleString
				return
			}
			verb := c.verb
			if verb == 'w' { // handleMethods passes %w on as %v
				verb = 'v'
			}
			s = percentBangString + string(rune(verb)) + panicString + method + " method: " + Sprint(err) + ")"
		}
	}()
	if e, ok := c.arg.(error); ok {
		return padded(e.Error(), c.wid, c.minus)
	}
	method = "String"
	return padded(c.arg.(Stringer).String(), c.wid, c.minus)
}

// fastInt64 and fastUint64 format v for verb as fmtInteger does without
// flags, or return "" for verbs they leave to Go's code.
func fastInt64(v int64, verb byte, prec int) string {
	switch {
	case prec >= 0:
	case verb == 'd' || verb == 'v':
		return strconv.FormatInt(v, 10)
	case verb == 'x':
		return strconv.FormatInt(v, 16)
	case verb == 'X':
		return upperHex(strconv.FormatInt(v, 16))
	}
	return ""
}

func fastUint64(v uint64, verb byte, prec int) string {
	switch {
	case prec >= 0:
	case verb == 'd' || verb == 'v':
		return strconv.FormatUint(v, 10)
	case verb == 'x':
		return strconv.FormatUint(v, 16)
	case verb == 'X':
		return upperHex(strconv.FormatUint(v, 16))
	}
	return ""
}

// fastFloat formats v as fmtFloat does without the + and space flags, or
// returns "" for verbs it leaves to Go's code.
func fastFloat(v float64, size int, verb byte, prec int) string {
	switch verb {
	case 'v':
		verb = 'g'
	case 'g', 'G':
	case 'f', 'e', 'E':
		if prec < 0 {
			prec = 6
		}
	case 'F':
		verb = 'f'
		if prec < 0 {
			prec = 6
		}
	default:
		return ""
	}
	if size == 64 {
		if verb == 'g' && prec < 0 {
			return fmtShortest(v)
		}
		if verb == 'f' {
			if s := fmtFixed(v, prec); s != "" {
				return s
			}
		}
	}
	return strconv.FormatFloat(v, verb, prec, size) // "+Inf" too, as fmt prints it
}

// jsSprintf is Sprintf(format, a...) for the verbs %v %d %s %t %x %X %f
// %F of strings, booleans, integers and float64s, with a format parsed
// once (runtime/src/fmt.ts); ok is false for anything else.
func jsSprintf(format string, a []any) (s string, ok bool)

// indexPercent is the index of the first % in s from index from on, or -1.
func indexPercent(s string, from int) int

// fmtInt and fmtUint format an int or a uint, which are JS numbers (not
// BigInts like int64), for verb as fmtInteger does without flags: %d and
// %v in base 10, %x and %X in base 16. They return "" for other verbs and
// with a precision (prec >= 0).
func fmtInt(v int, verb byte, prec int) string
func fmtUint(v uint, verb byte, prec int) string

// fmtFixed is strconv.FormatFloat(v, 'f', prec, 64), or "" for the values
// that need strconv's own code.
func fmtFixed(v float64, prec int) string

// fmtShortest is strconv.FormatFloat(v, 'g', -1, 64).
func fmtShortest(v float64) string

func upperHex(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'f' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// padded pads s with spaces to wid runes, on the right if minus.
func padded(s string, wid int, minus bool) string {
	if wid <= 0 {
		return s
	}
	n := wid - utf8.RuneCountInString(s)
	if n <= 0 {
		return s
	}
	pad := "                "
	for len(pad) < n {
		pad += pad
	}
	if minus {
		return s + pad[:n]
	}
	return pad[:n] + s
}
