package lower

import (
	"slices"
	"strings"
	"testing"
)

// TestByteBools checks which []bool variables byteBools backs with a
// Uint8Array. A variable it takes by mistake hands its bytes, 0 and 1, to
// code that expects booleans.
func TestByteBools(t *testing.T) {
	for _, c := range []struct {
		body  string
		want  string // the variables backed by bytes
		makes int    // the make calls of those
	}{
		{`b := make([]bool, n); b[0] = true; _ = b[1] && len(b) > 1`, "b", 1},
		{`var b []bool; b = make([]bool, n); b[0] = true`, "b", 1},
		{`var b = make([]bool, n); b[0] = !b[1]`, "b", 1},
		{`b := make([]bool, n); for i := range b { b[i] = true }`, "b", 1},
		{`b := make([]bool, n); b = nil; _ = cap(b)`, "b", 1},
		{`b := make([]bool, n); b = make([]bool, 2*n); b[0] = true`, "b", 2},
		{`b, k := make([]bool, n), 0; b[k] = true`, "b", 1},
		{`b, c := make([]bool, n), make([]bool, n); b[0], c[0] = c[1], b[1]`, "b c", 2},
		// Variables of other types next to one.
		{`var k int; x := make([][]bool, n); x[0] = nil; b := make([]bool, n); b[k] = true`, "b", 1},

		// The value leaves the variable.
		{`b := make([]bool, n); for _, x := range b { _ = x }`, "", 0},
		{`b := make([]bool, n); use(b)`, "", 0},
		{`var b = make([]bool, n); use(b)`, "", 0},
		{`b := make([]bool, n); c := b; _ = c`, "", 0},
		{`b := make([]bool, n); _ = b[1:]`, "", 0},
		{`b := make([]bool, n); b = append(b, true)`, "", 0},
		{`b := make([]bool, n); q := &b[0]; _ = q`, "", 0},
		{`b := make([]bool, n); q := &b; _ = q`, "", 0},
		{`b := make([]bool, n); _ = len(b) + len(other(b))`, "", 0},
		// It is not made by make([]bool, n) in the function.
		{`m := make(map[int]bool, n); m[0] = true; _ = len(m)`, "", 0},
		{`var x []int; x = nil; _ = x[n] + len(x)`, "", 0},
		{`b := []bool{true}; b[0] = false`, "", 0},
		{`b := mk(n); b[0] = true`, "", 0},
		{`b, err := mk2(n); b[0] = true; _ = err`, "", 0},
		{`b := g; b[0] = true`, "", 0},
		{`b := make([]bool, n); b = g; b[0] = true`, "", 0},
		{`p[0] = true; p = make([]bool, n); p[0] = true`, "", 0},
		{`p, k := make([]bool, n), 1; p[k] = true`, "", 0},
		{`g = make([]bool, n); g[0] = true`, "", 0},
	} {
		_, _, info, body := checkFunc(t, `
var g []bool

func use([]bool) {}

func other(b []bool) []bool { return b }

func mk(n int) []bool { return make([]bool, n) }

func mk2(n int) ([]bool, error) { return make([]bool, n), nil }
`, "n int, p []bool", c.body)
		vars, makes := byteBools(info, body)
		var names []string
		for v, ok := range vars {
			if ok {
				names = append(names, v.Name())
			}
		}
		slices.Sort(names)
		if got := strings.Join(names, " "); got != c.want {
			t.Errorf("%s: backed by bytes: %q, want %q", c.body, got, c.want)
		}
		n := 0
		for _, ok := range makes {
			if ok {
				n++
			}
		}
		if n != c.makes {
			t.Errorf("%s: %d make calls, want %d", c.body, n, c.makes)
		}
	}
}
