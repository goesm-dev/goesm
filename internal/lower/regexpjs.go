package lower

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"unicode"
)

// Regular expressions as the engine's RegExp. Package regexp, compiled from
// Go, is the largest part of a bundle that matches a pattern (60 KB gzipped
// with regexp/syntax's parser and fold tables and the three matching
// engines), while JavaScript has regular expressions built in. When every
// pattern a program compiles is known at compile time and its matches are
// the same as a RegExp's (translateRegexp), goesm parses each one here, at
// compile time, with Go's own regexp/syntax, and the program matches with a
// RegExp translated from it: the patched regexp.compile looks the pattern
// up (jsPattern, which the lowering writes) and find runs the RegExp
// (execJS in runtime/src/natives.ts). Go's parser and engines are then left
// out (regexpLeftOut). A program that compiles any other pattern keeps
// them, and matches every pattern with them.
//
// A RegExp matches what Go's regexp matches, with the same submatches,
// when the translation spells out every character class as code point
// ranges (Go's \s, \w and case folding are not JavaScript's), and the
// pattern stays within the part of the syntax where JavaScript's
// backtracking and Go's leftmost-first semantics agree: no capture group
// and no subexpression that can match the empty string under a repetition,
// where JavaScript resets captures in each iteration and stops an
// iteration that matches nothing. A Go string holds bytes, so the RegExp
// matches an ASCII string directly and otherwise the string decoded as Go
// decodes it (an invalid byte is U+FFFD, which is what a Go pattern sees
// too), and positions are mapped back to bytes.
//
// Go also guarantees that matching takes time linear in the input, which
// a backtracking engine does not: (a+)+b on a long run of a's backtracks
// exponentially, and \d+x unanchored rescans every run of digits from each
// of its positions. So only patterns whose backtracking cannot blow up are
// translated (safe): the pattern is deterministic (at each step of a match,
// the next character decides which part of the pattern it belongs to, so a
// failing alternative fails at once), or at least only finitely ambiguous
// (ambiguous), and a pattern that can start anywhere (not anchored by ^ or
// \A) cannot rescan: a repetition after which the match can still fail
// matches no character a match can start with, as in \$\{[a-z]+\}, so the
// search tries no start inside the run it scanned. A search then takes
// time linear in the input times the pattern's length.

// regexpEntry is the compile-time translation of a pattern, as jsPattern
// returns it to the regexp patch: the RegExp source, the number of
// capture groups, whether the literal prefix is the whole pattern, the
// groups' names separated by commas, and the literal prefix, separated by
// NUL characters (the prefix last, since it can hold any byte).
func regexpEntry(src string, nsub int, complete bool, names []string, prefix string) string {
	c := "0"
	if complete {
		c = "1"
	}
	return src + "\x00" + strconv.Itoa(nsub) + "\x00" + c + "\x00" + strings.Join(names[1:], ",") + "\x00" + prefix
}

// maxRegexpPositions bounds the characters of a translated pattern, its
// counted repetitions expanded.
const maxRegexpPositions = 2000

// translateRegexp returns the jsPattern entry of the pattern expr as
// regexp.Compile parses it, or ok == false if the program must keep Go's
// engine for it.
func translateRegexp(expr string) (entry string, ok bool) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return "", false // the error is Go's to report
	}
	tree, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return "", false
	}
	if !check(tree, false) {
		return "", false
	}
	if !(&rxAnalysis{}).safe(tree) {
		return "", false
	}
	var b strings.Builder
	jsRegexp(&b, tree)
	prefix, complete := re.LiteralPrefix()
	return regexpEntry(b.String(), re.NumSubexp(), complete, re.SubexpNames(), prefix), true
}

// check reports whether re has no capture group under a repetition that
// can match more than once (inLoop), no repeated subexpression that can
// match the empty string, and no alternation with two alternatives that
// can (whose epsilon paths would multiply).
func check(re *syntax.Regexp, inLoop bool) bool {
	switch re.Op {
	case syntax.OpCapture:
		if inLoop {
			return false
		}
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		if re.Op == syntax.OpRepeat && re.Min == 1 && re.Max == 1 {
			break
		}
		if nullable(re.Sub[0], true) {
			return false
		}
		if re.Op != syntax.OpQuest && (re.Op != syntax.OpRepeat || re.Max != 1) {
			inLoop = true
		}
	case syntax.OpAlternate:
		n := 0
		for _, sub := range re.Sub {
			if nullable(sub, true) {
				n++
			}
		}
		if n > 1 {
			return false
		}
	}
	for _, sub := range re.Sub {
		if !check(sub, inLoop) {
			return false
		}
	}
	return true
}

// nullable reports whether re can match the empty string; with
// assertions, also through empty-width assertions.
func nullable(re *syntax.Regexp, assertions bool) bool {
	switch re.Op {
	case syntax.OpEmptyMatch:
		return true
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return assertions
	case syntax.OpLiteral:
		return len(re.Rune) == 0
	case syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpRepeat:
		return re.Min == 0 || nullable(re.Sub[0], assertions)
	case syntax.OpCapture, syntax.OpPlus:
		return nullable(re.Sub[0], assertions)
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !nullable(sub, assertions) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if nullable(sub, assertions) {
				return true
			}
		}
		return false
	}
	return false // characters, OpNoMatch
}

// rxAnalysis computes the Glushkov automaton of a pattern: its positions
// are the characters it matches, each with the set of runes it accepts,
// and follow[p] are the positions that can come right after p. The
// pattern is deterministic if the first positions, and the positions
// following any one position, accept disjoint sets of runes.
type rxAnalysis struct {
	sets   [][]rune // position -> rune ranges (pairs, as syntax's classes)
	follow []map[int]bool
	loop   []bool // the position is under a repetition without an upper bound
	tooBig bool
}

// rxInfo is what the automaton needs of a subexpression: whether it can
// match the empty string (with assertions between the characters it
// matches, and without), its first positions and its last positions (also
// the last ones without an assertion after them).
type rxInfo struct {
	nullable, cleanNullable bool
	first, last, cleanLast  []int
}

func (a *rxAnalysis) pos(set []rune, loop bool) rxInfo {
	if len(a.sets) >= maxRegexpPositions {
		a.tooBig = true
	}
	p := len(a.sets)
	a.sets = append(a.sets, set)
	a.follow = append(a.follow, map[int]bool{})
	a.loop = append(a.loop, loop)
	return rxInfo{first: []int{p}, last: []int{p}, cleanLast: []int{p}}
}

func (a *rxAnalysis) link(from, to []int) {
	for _, p := range from {
		for _, q := range to {
			a.follow[p][q] = true
		}
	}
}

func (a *rxAnalysis) concat(x, y rxInfo) rxInfo {
	a.link(x.last, y.first)
	r := rxInfo{nullable: x.nullable && y.nullable, cleanNullable: x.cleanNullable && y.cleanNullable}
	r.first = x.first
	if x.nullable {
		r.first = union(x.first, y.first)
	}
	r.last = y.last
	if y.nullable {
		r.last = union(y.last, x.last)
	}
	r.cleanLast = y.cleanLast
	if y.cleanNullable {
		r.cleanLast = union(y.cleanLast, x.cleanLast)
	}
	return r
}

func (a *rxAnalysis) alt(x, y rxInfo) rxInfo {
	return rxInfo{
		nullable:      x.nullable || y.nullable,
		cleanNullable: x.cleanNullable || y.cleanNullable,
		first:         union(x.first, y.first),
		last:          union(x.last, y.last),
		cleanLast:     union(x.cleanLast, y.cleanLast),
	}
}

func union(x, y []int) []int {
	r := append([]int(nil), x...)
	for _, q := range y {
		found := false
		for _, p := range x {
			found = found || p == q
		}
		if !found {
			r = append(r, q)
		}
	}
	return r
}

var empty = rxInfo{nullable: true, cleanNullable: true}

// build adds the positions of re, with each counted repetition expanded,
// and returns its rxInfo.
func (a *rxAnalysis) build(re *syntax.Regexp, loop bool) rxInfo {
	if a.tooBig {
		return empty
	}
	switch re.Op {
	case syntax.OpNoMatch:
		return rxInfo{}
	case syntax.OpEmptyMatch:
		return empty
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return rxInfo{nullable: true}
	case syntax.OpLiteral:
		r := empty
		for _, c := range re.Rune {
			r = a.concat(r, a.pos(literalSet(c, re.Flags&syntax.FoldCase != 0), loop))
		}
		return r
	case syntax.OpCharClass:
		return a.pos(re.Rune, loop)
	case syntax.OpAnyCharNotNL:
		return a.pos([]rune{0, '\n' - 1, '\n' + 1, unicode.MaxRune}, loop)
	case syntax.OpAnyChar:
		return a.pos([]rune{0, unicode.MaxRune}, loop)
	case syntax.OpCapture:
		return a.build(re.Sub[0], loop)
	case syntax.OpConcat:
		r := empty
		for _, sub := range re.Sub {
			r = a.concat(r, a.build(sub, loop))
		}
		return r
	case syntax.OpAlternate:
		r := rxInfo{}
		for _, sub := range re.Sub {
			r = a.alt(r, a.build(sub, loop))
		}
		return r
	case syntax.OpQuest:
		return a.alt(empty, a.build(re.Sub[0], loop))
	case syntax.OpStar, syntax.OpPlus:
		x := a.build(re.Sub[0], true)
		a.link(x.last, x.first)
		if re.Op == syntax.OpStar {
			return a.alt(empty, x)
		}
		return x
	case syntax.OpRepeat:
		// x{n,m} is n copies of x followed by m-n nested optional ones,
		// x{n,} n-1 copies followed by x+ (x* for n = 0).
		r := empty
		n := re.Min
		if re.Max == -1 && n > 0 {
			n--
		}
		for i := 0; i < n && !a.tooBig; i++ {
			r = a.concat(r, a.build(re.Sub[0], loop))
		}
		if re.Max == -1 {
			star := &syntax.Regexp{Op: syntax.OpPlus, Sub: re.Sub[:1]}
			if re.Min == 0 {
				star.Op = syntax.OpStar
			}
			return a.concat(r, a.build(star, loop))
		}
		var opt func(k int) rxInfo
		opt = func(k int) rxInfo {
			if k == 0 || a.tooBig {
				return empty
			}
			x := a.build(re.Sub[0], loop)
			return a.alt(empty, a.concat(x, opt(k-1)))
		}
		return a.concat(r, opt(re.Max-re.Min))
	}
	a.tooBig = true
	return empty
}

// safe reports whether matching re by backtracking takes linear time
// (see the comment at the top of the file).
func (a *rxAnalysis) safe(re *syntax.Regexp) bool {
	root := a.build(re, false)
	if a.tooBig {
		return false
	}
	if !anchored(re) {
		ends := map[int]bool{}
		for _, p := range root.cleanLast {
			ends[p] = true
		}
		var starts []rune // the runes a match can start with
		for _, p := range root.first {
			starts = append(starts, a.sets[p]...)
		}
		for p, loop := range a.loop {
			if loop && !ends[p] && overlaps(a.sets[p], starts) {
				return false
			}
		}
	}
	// The automaton's states: 0 the start, p+1 position p.
	n := len(a.sets) + 1
	succ := make([][]int, n)
	succ[0] = plus1(root.first)
	deterministic := a.disjoint(root.first)
	for p := range a.sets {
		var f []int
		for q := range a.follow[p] {
			f = append(f, q)
		}
		deterministic = deterministic && a.disjoint(f)
		succ[p+1] = plus1(f)
	}
	if deterministic {
		return true
	}
	if n > maxAmbiguityStates {
		return false
	}
	return !a.ambiguous(succ)
}

func plus1(ps []int) []int {
	r := make([]int, len(ps))
	for i, p := range ps {
		r[i] = p + 1
	}
	return r
}

// maxAmbiguityStates bounds the automata whose ambiguity is computed.
const maxAmbiguityStates = 100

// ambiguous reports whether the automaton (succ, with the runes of state q
// in a.sets[q-1]) lets a backtracking matcher take more than linear time
// on some input: whether it is exponentially ambiguous, a state p having
// two different cycles on the same string, or (approximately) infinitely
// ambiguous, two different states p and q on cycles with a string leading
// from p both back to p and to q. Neither is the case for
// ^(?:(\d+)h)?(?:(\d+)m)?$, which is not deterministic (both groups start
// with a digit) but whose wrong guess fails at the h. Weideman et al.,
// "Analyzing Matching Time Behavior of Backtracking Regular Expression
// Matchers by Using Ambiguity of NFA" (2016).
func (a *rxAnalysis) ambiguous(succ [][]int) bool {
	n := len(succ)
	same := func(x, y int) bool { return overlaps(a.sets[x-1], a.sets[y-1]) }
	// The pair automaton: (x, y) -> (x', y') where both runs read a rune
	// both x' and y' accept.
	pairSucc := func(v int) []int {
		x, y := v/n, v%n
		var r []int
		for _, x2 := range succ[x] {
			for _, y2 := range succ[y] {
				if same(x2, y2) {
					r = append(r, x2*n+y2)
				}
			}
		}
		return r
	}
	reach := func(from int, edges func(int) []int, size int) []bool {
		seen := make([]bool, size)
		stack := append([]int(nil), edges(from)...)
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !seen[v] {
				seen[v] = true
				stack = append(stack, edges(v)...)
			}
		}
		return seen
	}
	onCycle := make([]bool, n)
	for p := 1; p < n; p++ {
		onCycle[p] = reach(p, func(v int) []int { return succ[v] }, n)[p]
	}
	for p := 1; p < n; p++ {
		if !onCycle[p] {
			continue
		}
		from := reach(p*n+p, pairSucc, n*n)
		for v, ok := range from {
			x, y := v/n, v%n
			if !ok || x == y {
				continue
			}
			// Two runs from p part on the same string.
			if reach(v, pairSucc, n*n)[p*n+p] {
				return true // and meet again at p
			}
			if x == p && onCycle[y] {
				return true
			}
		}
	}
	return false
}

func (a *rxAnalysis) disjoint(ps []int) bool {
	for i, p := range ps {
		for _, q := range ps[i+1:] {
			if overlaps(a.sets[p], a.sets[q]) {
				return false
			}
		}
	}
	return true
}

func overlaps(x, y []rune) bool {
	for i := 0; i+1 < len(x); i += 2 {
		for j := 0; j+1 < len(y); j += 2 {
			if x[i] <= y[j+1] && y[j] <= x[i+1] {
				return true
			}
		}
	}
	return false
}

// anchored reports whether every match of re starts at the beginning of
// the text.
func anchored(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpBeginText:
		return true
	case syntax.OpCapture:
		return anchored(re.Sub[0])
	case syntax.OpConcat:
		return len(re.Sub) > 0 && anchored(re.Sub[0])
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if !anchored(sub) {
				return false
			}
		}
		return true
	}
	return false
}

// literalSet is the rune ranges a literal rune matches: the rune, or with
// case folding its orbit, as Go's compiler matches it.
func literalSet(r rune, fold bool) []rune {
	if !fold {
		return []rune{r, r}
	}
	var rs []rune
	for f := r; ; {
		rs = append(rs, f)
		if f = unicode.SimpleFold(f); f == r {
			break
		}
	}
	var set []rune
	for _, c := range rs {
		set = append(set, c, c)
	}
	return set
}

// jsRegexp writes re as the source of a JavaScript RegExp over UTF-16
// code units (see jsSet).
func jsRegexp(b *strings.Builder, re *syntax.Regexp) {
	switch re.Op {
	case syntax.OpNoMatch:
		b.WriteString("[]")
	case syntax.OpEmptyMatch:
		b.WriteString("(?:)")
	case syntax.OpLiteral:
		for _, c := range re.Rune {
			jsSet(b, literalSet(c, re.Flags&syntax.FoldCase != 0))
		}
	case syntax.OpCharClass:
		jsSet(b, re.Rune)
	case syntax.OpAnyCharNotNL:
		jsSet(b, []rune{0, '\n' - 1, '\n' + 1, unicode.MaxRune})
	case syntax.OpAnyChar:
		jsSet(b, []rune{0, unicode.MaxRune})
	case syntax.OpBeginLine:
		b.WriteString(`(?<![^\n])`)
	case syntax.OpEndLine:
		b.WriteString(`(?![^\n])`)
	case syntax.OpBeginText:
		b.WriteString(`^`)
	case syntax.OpEndText:
		b.WriteString(`$`)
	case syntax.OpWordBoundary:
		b.WriteString(`\b`)
	case syntax.OpNoWordBoundary:
		b.WriteString(`\B`)
	case syntax.OpCapture:
		b.WriteString("(")
		jsRegexp(b, re.Sub[0])
		b.WriteString(")")
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		jsAtom(b, re.Sub[0])
		switch re.Op {
		case syntax.OpStar:
			b.WriteString("*")
		case syntax.OpPlus:
			b.WriteString("+")
		case syntax.OpQuest:
			b.WriteString("?")
		default:
			switch {
			case re.Max == -1:
				fmt.Fprintf(b, "{%d,}", re.Min)
			case re.Max == re.Min:
				fmt.Fprintf(b, "{%d}", re.Min)
			default:
				fmt.Fprintf(b, "{%d,%d}", re.Min, re.Max)
			}
		}
		if re.Flags&syntax.NonGreedy != 0 {
			b.WriteString("?")
		}
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if sub.Op == syntax.OpAlternate {
				b.WriteString("(?:")
				jsRegexp(b, sub)
				b.WriteString(")")
			} else {
				jsRegexp(b, sub)
			}
		}
	case syntax.OpAlternate:
		for i, sub := range re.Sub {
			if i > 0 {
				b.WriteString("|")
			}
			jsRegexp(b, sub)
		}
	}
}

// jsAtom writes re so that a quantifier applies to all of it.
func jsAtom(b *strings.Builder, re *syntax.Regexp) {
	switch {
	case re.Op == syntax.OpCharClass, re.Op == syntax.OpAnyChar, re.Op == syntax.OpAnyCharNotNL, re.Op == syntax.OpCapture,
		re.Op == syntax.OpLiteral && len(re.Rune) == 1:
		jsRegexp(b, re) // jsSet writes an atom
	default:
		b.WriteString("(?:")
		jsRegexp(b, re)
		b.WriteString(")")
	}
}

// jsSet writes an atom that matches one rune of the set of rune ranges
// set, in a string of UTF-16 code units: the runes of the Basic
// Multilingual Plane as a class, the others as their surrogate pairs. The
// RegExp has no u flag, with which V8 misses matches such as that of
// [^\n]{2}$ in "ak\U0001F600" (it seems to measure the pattern in code units
// without counting pairs), so the pairs are spelled out. The surrogates
// themselves are left out: a decoded Go string has none alone.
func jsSet(b *strings.Builder, set []rune) {
	var bmp []string
	var pairs []string
	for i := 0; i+1 < len(set); i += 2 {
		lo, hi := set[i], set[i+1]
		for _, r := range [][2]rune{{lo, min(hi, 0xd7ff)}, {max(lo, 0xe000), min(hi, 0xffff)}} {
			if r[0] <= r[1] {
				s := jsUnit(r[0])
				if r[1] != r[0] {
					s += "-" + jsUnit(r[1])
				}
				bmp = append(bmp, s)
			}
		}
		if hi < 0x10000 {
			continue
		}
		lo = max(lo, 0x10000)
		hi1, lo1 := utf16Pair(lo)
		hi2, lo2 := utf16Pair(hi)
		if hi1 == hi2 {
			pairs = append(pairs, jsUnit(hi1)+jsUnitRange(lo1, lo2))
			continue
		}
		pairs = append(pairs, jsUnit(hi1)+jsUnitRange(lo1, 0xdfff))
		if hi1+1 <= hi2-1 {
			pairs = append(pairs, "["+jsUnit(hi1+1)+"-"+jsUnit(hi2-1)+"]"+jsUnitRange(0xdc00, 0xdfff))
		}
		pairs = append(pairs, jsUnit(hi2)+jsUnitRange(0xdc00, lo2))
	}
	class := "[" + strings.Join(bmp, "") + "]"
	if len(bmp) == 1 && !strings.Contains(bmp[0], "-") {
		class = bmp[0]
	}
	if len(pairs) == 0 {
		b.WriteString(class)
		return
	}
	if len(bmp) > 0 {
		pairs = append([]string{class}, pairs...)
	}
	b.WriteString("(?:" + strings.Join(pairs, "|") + ")")
}

// utf16Pair returns the surrogate pair of the rune r above U+FFFF.
func utf16Pair(r rune) (rune, rune) {
	r -= 0x10000
	return 0xd800 + r>>10, 0xdc00 + r&0x3ff
}

func jsUnitRange(lo, hi rune) string {
	if lo == hi {
		return jsUnit(lo)
	}
	return "[" + jsUnit(lo) + "-" + jsUnit(hi) + "]"
}

// jsUnit spells the UTF-16 code unit c in a RegExp: ASCII letters and
// digits as they are, everything else escaped, so that the source is
// ASCII.
func jsUnit(c rune) string {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return string(c)
	}
	return fmt.Sprintf(`\u%04x`, c)
}
