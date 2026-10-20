//go:build js

package jscompat

import (
	"errors"
	"syscall/js"
)

// The engine's Math functions, whose last bit may differ from the math
// package's (math.Cos is Go's own algorithm, here and natively).

// Cos returns JavaScript's Math.cos(x).
//
//goesm:import "./jscompat.ts" cos
func Cos(x float64) float64 { return mathFn("cos", x) }

// Sin returns JavaScript's Math.sin(x).
//
//goesm:import "./jscompat.ts" sin
func Sin(x float64) float64 { return mathFn("sin", x) }

// Tan returns JavaScript's Math.tan(x).
//
//goesm:import "./jscompat.ts" tan
func Tan(x float64) float64 { return mathFn("tan", x) }

// Exp returns JavaScript's Math.exp(x).
//
//goesm:import "./jscompat.ts" exp
func Exp(x float64) float64 { return mathFn("exp", x) }

// Log returns JavaScript's Math.log(x).
//
//goesm:import "./jscompat.ts" log
func Log(x float64) float64 { return mathFn("log", x) }

// Pow returns JavaScript's Math.pow(x, y).
//
//goesm:import "./jscompat.ts" pow
func Pow(x, y float64) float64 { return mathFn("pow", x, y) }

// Atan2 returns JavaScript's Math.atan2(y, x).
//
//goesm:import "./jscompat.ts" atan2
func Atan2(y, x float64) float64 { return mathFn("atan2", y, x) }

func mathFn(name string, args ...any) float64 {
	return js.Global().Get("Math").Call(name, args...).Float()
}

// Normalize returns the Unicode normalization form ("NFC", "NFD", "NFKC"
// or "NFKD") of s, as JavaScript's String.prototype.normalize computes it.
// It panics for another form.
//
//goesm:import "./jscompat.ts" normalize
func Normalize(s, form string) string {
	return jsString(s).Call("normalize", form).String()
}

// RegExp is a JavaScript regular expression, with what Go's regexp lacks:
// lookahead, lookbehind, backreferences and named groups in JavaScript's
// syntax. A constant pattern that Go's regexp accepts needs none of this:
// goesm already matches it with RegExp when that is safe.
//
// Its methods take and return Go strings. A RegExp is safe for concurrent
// use.
type RegExp struct {
	re js.Value
}

// CompileRegExp compiles pattern with flags ("i", "m", "s", "u", "v" and
// "d"; "g" and "y" are ignored, each method knows whether it finds one
// match or all). A syntax error is returned as a js.Error.
func CompileRegExp(pattern, flags string) (*RegExp, error) {
	re, err := compile(pattern, flags)
	if err != nil {
		return nil, err
	}
	return &RegExp{re}, nil
}

// MustCompileRegExp is CompileRegExp that panics on a syntax error.
func MustCompileRegExp(pattern, flags string) *RegExp {
	re, err := CompileRegExp(pattern, flags)
	if err != nil {
		panic("jscompat: MustCompileRegExp(" + pattern + "): " + err.Error())
	}
	return re
}

// String returns the source of the pattern.
func (r *RegExp) String() string { return source(r.re) }

// MatchString reports whether s contains a match.
func (r *RegExp) MatchString(s string) bool { return test(r.re, s) }

// FindStringSubmatch returns the leftmost match and its groups, "" for a
// group that did not take part; nil if there is no match.
func (r *RegExp) FindStringSubmatch(s string) []string { return exec(r.re, s) }

// FindAllString returns the successive matches, at most n of them if n >=
// 0; nil if there is none.
func (r *RegExp) FindAllString(s string, n int) []string {
	m := matchAll(r.re, s, n)
	if len(m) == 0 {
		return nil
	}
	return m
}

// ReplaceAllString replaces every match with repl, in which JavaScript's
// replacement patterns stand for the match: $& the match, $1 a group, $<name>
// a named group, $$ a dollar sign.
func (r *RegExp) ReplaceAllString(s, repl string) string { return replace(r.re, s, repl) }

//goesm:import "./jscompat.ts" compile
func compile(pattern, flags string) (js.Value, error) { return compileWasm(pattern, flags) }

//goesm:import "./jscompat.ts" test
func test(re js.Value, s string) bool { return re.Get("one").Call("test", s).Bool() }

//goesm:import "./jscompat.ts" exec
func exec(re js.Value, s string) []string {
	m := re.Get("one").Call("exec", s)
	if m.IsNull() {
		return nil
	}
	out := make([]string, m.Length())
	for i := range out {
		if g := m.Index(i); !g.IsUndefined() {
			out[i] = g.String()
		}
	}
	return out
}

//goesm:import "./jscompat.ts" matchAll
func matchAll(re js.Value, s string, n int) []string {
	all := re.Get("all")
	all.Set("lastIndex", 0)
	var out []string
	for n < 0 || len(out) < n {
		m := all.Call("exec", s)
		if m.IsNull() {
			break
		}
		out = append(out, m.Index(0).String())
		if out[len(out)-1] == "" {
			all.Set("lastIndex", all.Get("lastIndex").Int()+1)
		}
	}
	return out
}

//goesm:import "./jscompat.ts" replace
func replace(re js.Value, s, repl string) string {
	return jsString(s).Call("replace", re.Get("all"), repl).String()
}

//goesm:import "./jscompat.ts" source
func source(re js.Value) string { return re.Get("one").Get("source").String() }

// jsString wraps s in a String object, whose methods Go's syscall/js can
// call (it panics on a primitive string).
func jsString(s string) js.Value { return js.Global().Get("String").New(s) }

// compileWasm builds the pair jscompat.ts's compile returns, for Go's
// js/wasm port.
func compileWasm(pattern, flags string) (re js.Value, err error) {
	defer func() {
		if x := recover(); x != nil {
			if e, ok := x.(js.Error); ok {
				err = e
				return
			}
			err = errors.New("jscompat: " + pattern + ": invalid regular expression")
		}
	}()
	one := ""
	for _, f := range flags {
		if f != 'g' && f != 'y' {
			one += string(f)
		}
	}
	ctor := js.Global().Get("RegExp")
	pair := js.Global().Get("Object").New()
	pair.Set("one", ctor.New(pattern, one))
	pair.Set("all", ctor.New(pattern, one+"g"))
	return pair, nil
}
