package lower

import (
	"regexp"
	"strings"
	"testing"
)

// TestTranslateRegexp checks which patterns translateRegexp translates to a
// RegExp and what the RegExp is; testdata/programs/regexpjs compares the
// matches with native Go's.
func TestTranslateRegexp(t *testing.T) {
	for _, c := range []struct {
		expr, js string // js "" when the pattern keeps Go's engine
	}{
		{`^(\d{4})-(\d{1,2})-(\d{1,2})$`, `^([0-9]{4})\u002d([0-9]{1,2})\u002d([0-9]{1,2})$`},
		{`\d+`, `[0-9]+`},
		{`(?i)k`, `[Kk\u212a]`},
		{`\s`, `[\u0009-\u000a\u000c-\u000d\u0020]`}, // not JavaScript's \s
		{`.`, `(?:[\u0000-\u0009\u000b-\ud7ff\ue000-\uffff]|\ud800[\udc00-\udfff]|[\ud801-\udbfe][\udc00-\udfff]|\udbff[\udc00-\udfff])`},
		{`\x{1F600}+`, `(?:\ud83d\ude00)+`},
		{`(?m)^a$`, `(?<![^\n])a(?![^\n])`},
		{`(a)?b`, `(a)?b`},
		{`a|ab`, `a(?:(?:)|b)`}, // as Go's parser factors it
		{`(?U)a+`, `a+?`},
		// Not deterministic, but a wrong guess fails at the h.
		{`^(?:(\d+)h)?(?:(\d+)m)?$`, `^(?:([0-9]+)h)?(?:([0-9]+)m)?$`},
		// Unanchored, but no start of a match is inside [a-z]+.
		{`\$\{([a-z]+)\}`, `\u0024\u007b([a-z]+)\u007d`},

		{`(?:(a)|b)+`, ""},     // JavaScript resets the capture in each iteration
		{`(?:|a)?`, ""},        // an iteration that matches nothing
		{`(a+)+b`, ""},         // exponential backtracking
		{`\d+\d+$`, ""},        // polynomial backtracking
		{`^(\S+)\s+(.*)$`, ""}, // polynomial backtracking on spaces before a \n
		{`\d+x`, ""},           // unanchored: each digit of a run starts a rescan
		{`(\w+)@(\w+)`, ""},    // likewise
		{`a(`, ""},             // Go reports the error
	} {
		e, ok := translateRegexp(c.expr)
		if js, _, _ := strings.Cut(e, "\x00"); js != c.js || ok != (c.js != "") {
			t.Errorf("%s: got %q (%v), want %q", c.expr, js, ok, c.js)
		}
	}
}

// TestRegexpEntry checks the fields jsPattern passes to the regexp patch's
// compileJS.
func TestRegexpEntry(t *testing.T) {
	expr := `ab(?P<x>c)(d)(?P<y>e)`
	e, ok := translateRegexp(expr)
	if !ok {
		t.Fatal("not translated")
	}
	re := regexp.MustCompile(expr)
	prefix, complete := re.LiteralPrefix()
	want := regexpEntry("ab(c)(d)(e)", 3, complete, re.SubexpNames(), prefix)
	if e != want || prefix != "abcde" || !strings.HasSuffix(e, "\x003\x001\x00x,,y\x00abcde") {
		t.Errorf("got %q, want %q", e, want)
	}
}
