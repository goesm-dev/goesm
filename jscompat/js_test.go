//go:build js

package jscompat

import (
	"slices"
	"testing"
)

// TestEngine runs with Go's js/wasm port (GOOS=js GOARCH=wasm go test, with
// go_js_wasm_exec); goesm runs the same calls in testdata/jsimport.
func TestEngine(t *testing.T) {
	if got := Normalize("ｶﾞ①ﬁ", "NFKC"); got != "ガ1fi" {
		t.Errorf("Normalize = %q", got)
	}
	if Pow(2, 10) != 1024 {
		t.Error("Pow")
	}
	re := MustCompileRegExp(`(?<=¥)(\d+)(?!\d*円)`, "")
	if !re.MatchString("¥1200") || re.MatchString("1200") || re.String() != `(?<=¥)(\d+)(?!\d*円)` {
		t.Error("MatchString")
	}
	if got := MustCompileRegExp(`(a)|(b)`, "").FindStringSubmatch("b"); !slices.Equal(got, []string{"b", "", "b"}) {
		t.Errorf("FindStringSubmatch = %q", got)
	}
	if got := MustCompileRegExp(`x*`, "").FindAllString("axx", -1); !slices.Equal(got, []string{"", "xx", ""}) {
		t.Errorf("FindAllString = %q", got)
	}
	if got := MustCompileRegExp(`(\p{L})\1`, "u").FindAllString("ああいいう", 1); !slices.Equal(got, []string{"ああ"}) {
		t.Errorf("FindAllString n = %q", got)
	}
	if got := MustCompileRegExp(`(?<y>\d{4})-(?<m>\d\d)`, "").ReplaceAllString("2026-10", "$<m>/$<y>"); got != "10/2026" {
		t.Errorf("ReplaceAllString = %q", got)
	}
	if _, err := CompileRegExp(`(`, ""); err == nil {
		t.Error("CompileRegExp accepted (")
	}
}
