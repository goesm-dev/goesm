//go:build goesm

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of regexp's regexp.go: a program whose patterns are all
// known at compile time, and translated there, matches with the engine's
// RegExp (internal/lower/regexpjs.go). compile then finds the pattern's
// translation with jsPattern, whose body the lowering writes, and find
// runs the RegExp (exec.go). replaceAll and matches convert a []byte to a
// string once for all the searches they make.
package regexp

import (
	"iter"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// Regexp is the representation of a compiled regular expression.
// A Regexp is safe for concurrent use by multiple goroutines,
// except for configuration methods, such as [Regexp.Longest].
type Regexp struct {
	expr           string       // as passed to Compile
	prog           *syntax.Prog // compiled program
	onepass        *onePassProg // onepass program or nil
	numSubexp      int
	maxBitStateLen int
	subexpNames    []string
	prefix         string         // required prefix in unanchored matches
	prefixBytes    []byte         // prefix, as a []byte
	prefixRune     rune           // first rune in prefix
	prefixEnd      uint32         // pc for last rune in prefix
	mpool          int            // pool for machines
	matchcap       int            // size of recorded match lengths
	prefixComplete bool           // prefix is the entire regexp
	cond           syntax.EmptyOp // empty-width conditions required at start of match
	minInputLen    int            // minimum length of the input in bytes

	// This field can be modified by the Longest method,
	// but it is otherwise read-only.
	longest bool // whether regexp prefers leftmost-longest match

	js string // the source of the RegExp it matches with, or ""
}

// compile uses the RegExp translated from expr if there is one.
//
//goesm:original compileGo
func compile(expr string, mode syntax.Flags, longest bool) (*Regexp, error) {
	if mode == syntax.Perl && !longest {
		if e := jsPattern(expr); e != "" {
			return compileJS(expr, e), nil
		}
	}
	return compileGo(expr, mode, longest)
}

// jsPattern returns the translation of the pattern expr (regexpEntry in
// internal/lower/regexpjs.go), or "" if it has none. The lowering writes its
// body for the patterns of the program.
func jsPattern(expr string) string {
	return ""
}

// compileJS returns the Regexp of expr matched with a RegExp, from the
// entry jsPattern returns for it.
func compileJS(expr, e string) *Regexp {
	src, e, _ := strings.Cut(e, "\x00")
	n, e, _ := strings.Cut(e, "\x00")
	complete, e, _ := strings.Cut(e, "\x00")
	names, prefix, _ := strings.Cut(e, "\x00")
	nsub := 0
	for i := 0; i < len(n); i++ {
		nsub = nsub*10 + int(n[i]-'0')
	}
	subexpNames := make([]string, nsub+1)
	for i := 1; i <= nsub; i++ {
		subexpNames[i], names, _ = strings.Cut(names, ",")
	}
	return &Regexp{
		expr:           expr,
		prog:           &syntax.Prog{NumCap: 2 * (nsub + 1)},
		numSubexp:      nsub,
		subexpNames:    subexpNames,
		prefix:         prefix,
		prefixComplete: complete == "1",
		matchcap:       2 * (nsub + 1),
		js:             src,
	}
}

func (re *Regexp) replaceAll(bsrc []byte, src string, nmatch int, repl func(dst []byte, m []int) []byte) []byte {
	lastMatchEnd := 0 // end position of the most recent match
	searchPos := 0    // position where we next look for a match
	var buf []byte
	var endPos int
	if bsrc != nil {
		endPos = len(bsrc)
	} else {
		endPos = len(src)
	}
	if nmatch > re.prog.NumCap {
		nmatch = re.prog.NumCap
	}

	// The RegExp matches a string, converted once.
	fb, fs := bsrc, src
	if re.js != "" && bsrc != nil {
		fb, fs = nil, string(bsrc)
	}

	var dstCap [2]int
	for searchPos <= endPos {
		a := re.find(nil, fb, fs, searchPos, nmatch, dstCap[:0])
		if len(a) == 0 {
			break // no more matches
		}

		// Copy the unmatched characters before this match.
		if bsrc != nil {
			buf = append(buf, bsrc[lastMatchEnd:a[0]]...)
		} else {
			buf = append(buf, src[lastMatchEnd:a[0]]...)
		}

		// Now insert a copy of the replacement string, but not for a
		// match of the empty string immediately after another match.
		// (Otherwise, we get double replacement for patterns that
		// match both empty and nonempty strings.)
		if a[1] > lastMatchEnd || a[0] == 0 {
			buf = repl(buf, a)
		}
		lastMatchEnd = a[1]

		// Advance past this match; always advance at least one character.
		var width int
		if bsrc != nil {
			_, width = utf8.DecodeRune(bsrc[searchPos:])
		} else {
			_, width = utf8.DecodeRuneInString(src[searchPos:])
		}
		if searchPos+width > a[1] {
			searchPos += width
		} else if searchPos+1 > a[1] {
			// This clause is only needed at the end of the input
			// string. In that case, DecodeRuneInString returns width=0.
			searchPos++
		} else {
			searchPos = a[1]
		}
	}

	// Copy the unmatched characters after the last match.
	if bsrc != nil {
		buf = append(buf, bsrc[lastMatchEnd:]...)
	} else {
		buf = append(buf, src[lastMatchEnd:]...)
	}

	return buf
}

// matches yields the location of successive matches in the input text.
// The input text is b if non-nil, otherwise s.
func (re *Regexp) matches(s string, b []byte, max, ncap int) iter.Seq[[]int] {
	return func(yield func([]int) bool) {
		if max == 0 {
			return
		}
		var end int
		if b == nil {
			end = len(s)
		} else {
			end = len(b)
		}
		// The RegExp matches a string, converted once.
		fb, fs := b, s
		if re.js != "" && b != nil {
			fb, fs = nil, string(b)
		}
		var matches []int
		for pos, prevMatchEnd := 0, -1; pos <= end; {
			matches = re.find(nil, fb, fs, pos, ncap, matches[:0])
			if len(matches) == 0 {
				break
			}

			accept := true
			if matches[1] == pos {
				// We've found an empty match.
				if matches[0] == prevMatchEnd {
					// We don't allow an empty match right
					// after a previous match, so ignore it.
					accept = false
				}
				var width int
				if b == nil {
					is := inputString{str: s}
					_, width = is.step(pos)
				} else {
					ib := inputBytes{str: b}
					_, width = ib.step(pos)
				}
				if width > 0 {
					pos += width
				} else {
					pos = end + 1
				}
			} else {
				pos = matches[1]
			}
			prevMatchEnd = matches[1]

			if accept {
				if !yield(re.pad(matches)) {
					return
				}
				if max > 0 {
					if max--; max == 0 {
						return
					}
				}
			}
		}
	}
}

// MustCompile is like [Compile] but panics if the expression cannot be parsed.
// It simplifies safe initialization of global variables holding compiled regular
// expressions.
func MustCompile(str string) *Regexp {
	regexp, err := Compile(str)
	if err != nil {
		panic(compileError(str, err))
	}
	return regexp
}

// compileError is MustCompile's panic, apart, so that a program matching
// with RegExps, whose patterns all compile, leaves it out with the call of
// err's Error method.
func compileError(str string, err error) string {
	return `regexp: Compile(` + quote(str) + `): ` + err.Error()
}
