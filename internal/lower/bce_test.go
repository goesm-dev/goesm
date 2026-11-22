package lower

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"
)

// TestInBoundsIndices checks which index expressions inBoundsIndices finds
// in range by construction. Those lose their bounds check, so a form it
// accepts by mistake makes an out-of-range index read undefined instead of
// panicking.
func TestInBoundsIndices(t *testing.T) {
	for _, c := range []struct {
		body string
		want bool // for every index expression of body
	}{
		// The forms of the doc comment.
		{`for i := range s { _ = s[i] }`, true},
		{`for i := range str { _ = str[i] }`, true},
		{`for i := 0; i < len(s); i++ { _ = s[i] }`, true},
		{`for i := 1; i < len(s); i += 2 { _ = s[i] }`, true},
		{`for i := len(t); i < len(s); i += len(t) { _ = s[i] }`, true},
		{`m := make([]int, 3); for i := 0; i < 3; i++ { _ = m[i] }`, true},
		{`m := make([]int, n); for i := 0; i < n; i++ { _ = m[i] }`, true},
		{`k := len(s); for i := range k { _ = s[i] }`, true},
		{`k := len(s); for i := 0; i < k; i++ { _ = s[i] }`, true},
		{`for i := range len(s) { _ = s[i] }`, true},
		{`var a [4]int; for i := 0; i < 4; i++ { _ = a[i] }`, true},
		{`var a [4]int; for i := range a { _ = a[i] }`, true},
		{`var a [4]int; for i := range 4 { _ = a[i] }`, true},
		{`for x := range n { _ = s[x%len(s)] }`, true},
		{`for x := range n { _ = str[(x+1)%len(str)] }`, true},
		{`for x := range n { _ = s[(x*2+cap(t))%len(s)] }`, true},
		// Loops inside loops, whether the outer loop has the form or not.
		{`for i := 0; i < len(s); i++ { for j := 0; j < len(t); j++ { _ = s[i]; _ = t[j] } }`, true},
		{`k := len(s); for i := 0; i < k; i++ { for j := 0; j < k; j++ { _ = s[i]; _ = s[j] } }`, true},
		{`m := make([]int, n); for i := 0; i < n; i++ { for j := 0; j < n; j++ { _ = m[i]; _ = m[j] } }`, true},
		{`for k := range ms { for i := range s { _ = s[i]; _ = k } }`, true},
		{`for range n { for i := range s { _ = s[i] } }`, true},
		{`for j, l := 0, n; j < l; j++ { for i := range s { _ = s[i] } }`, true},
		{`for j := -1; j < n; j++ { for i := range s { _ = s[i] } }`, true},
		{`for j := 0; j < n; j-- { for i := range s { _ = s[i] } }`, true},
		{`for j := 0; j <= n; j++ { for i := range s { _ = s[i] } }`, true},
		// Declarations of several variables from one call.
		{`x, y := pair(); for i := range s { _ = s[i] }; _, _ = x, y`, true},
		{`var x, y = pair(); for i := range s { _ = s[i] }; _, _ = x, y`, true},
		// Loops without an init statement.
		{`for i := range s { for n > 0 { n-- }; _ = s[i] }`, true},
		{`for i := range s { for ; n > 0; n-- { }; _ = s[i] }`, true},

		// The index can leave the range.
		{`for i := 0; i <= len(s); i++ { _ = s[i] }`, false},
		{`for i := -1; i < len(s); i++ { _ = s[i] }`, false},
		{`for i := 0; i < len(t); i++ { _ = s[i] }`, false},
		{`for i := 0; i < len(s); i++ { i++; _ = s[i] }`, false},
		{`for i := 0; i < len(s); i-- { _ = s[i] }`, false},
		{`for i := 0; i < len(s); i += n { _ = s[i] }`, false},
		{`for i := 0; i < len(s); i += n - 1 { _ = s[i] }`, false},
		{`for i := 0; i < len(s); i += n * 2 { _ = s[i] }`, false},
		{`for i, j := 0, 0; i < len(s); i++ { _ = s[i]; _ = j }`, false},
		{`for i := range t { _ = s[i] }`, false},
		{`for i := range n { _ = s[i] }`, false},
		{`for i := range s { _ = s[i-1] }`, false},
		{`for i := range s { _ = s[i]; i = 5 }`, false},
		{`for i := 0; i < len(s); i++ { for i = range t {}; _ = s[i] }`, false},
		{`for i := 0; i < len(s); i++ { for _, i = range t {}; _ = s[i] }`, false},
		{`for i := 0; i < cap(s); i++ { _ = s[i] }`, false},
		{`for x := range n { _ = s[x%cap(s)] }`, false},
		{`for x := range n { _ = s[(x+n)%len(s)] }`, false},
		{`for x := range n { _ = s[min(x, n)%len(s)] }`, false},
		{`for x := range n { _ = s[(x-1)%len(s)] }`, false},
		{`for x := range n { _ = s[x%len(t)] }`, false},
		{`_ = s[n%len(s)]`, false},
		// The slice or its length can change.
		{`for i := range s { s = t; _ = s[i] }`, false},
		{`for i := 0; i < len(s); i++ { s = s[1:]; _ = s[i] }`, false},
		{`p := &s; for i := range s { *p = nil; _ = s[i] }`, false},
		{`for i := range g { _ = g[i] }`, false},
		{`k := len(s); k--; for i := range k { _ = s[i] }`, false},
		{`k := len(s); s = s[1:]; for i := range k { _ = s[i] }`, false},
		{`k := len(s) + 1; for i := range k { _ = s[i] }`, false},
		// The bound is not the length the slice was made with.
		{`m := make([]int, 3); for i := 0; i < 4; i++ { _ = m[i] }`, false},
		{`m := make([]int, n); for i := 0; i < n+1; i++ { _ = m[i] }`, false},
		{`m := make([]int, n); n = 9; for i := 0; i < n; i++ { _ = m[i] }`, false},
		{`m := make([]int, n); for i := 0; i < len(t); i++ { _ = m[i] }`, false},
		{`m := grow(s, 3); for i := 0; i < 3; i++ { _ = m[i] }`, false},
		{`m := make([]int, n); m = s; for i := 0; i < n; i++ { _ = m[i] }`, false},
		{`var a [3]int; for i := 0; i < 4; i++ { _ = a[i] }`, false},
		{`var a [3]int; for i := range 4 { _ = a[i] }`, false},
		{`var a [3]int; var b [4]int; for i := range b { _ = a[i] }`, false},
		{`for i := range ms { _ = ms[i] }`, false},
	} {
		src, fset, info, body := bceFunc(t, c.body)
		in, _ := inBoundsIndices(info, body)
		n := 0
		ast.Inspect(body, func(x ast.Node) bool {
			if ix, ok := x.(*ast.IndexExpr); ok {
				n++
				if in[ix] != c.want {
					t.Errorf("%s: %s in bounds = %v, want %v", c.body, src[fset.Position(ix.Pos()).Offset:fset.Position(ix.End()).Offset], in[ix], c.want)
				}
			}
			return true
		})
		if n == 0 {
			t.Errorf("%s: no index expression", c.body)
		}
	}
}

// TestHdrLoops checks before which loops the lowering loads the headers of
// slices (hoistSliceHeaders): only slices that keep their header, before the
// outermost loop after their declaration, and outside function literals for
// outer.
func TestHdrLoops(t *testing.T) {
	for _, c := range []struct {
		body         string
		loops, outer string // loop number (in source order): slices
	}{
		{`for i := range s { _ = s[i] }`, "0:s", "s"},
		{`for i := 0; i < len(s); i++ { _ = s[i] }`, "0:s", "s"},
		{`for range n { for i := range s { _ = s[i] } }`, "0:s", "s"},
		{`for i := range s { for j := range t { _ = s[i]; _ = t[j] } }`, "0:s,t", "s t"},
		{`for range n { m := make([]int, 3); for i := range m { _ = m[i] } }`, "1:m", "m"},
		{`func() { for i := range s { _ = s[i] } }()`, "0:s", ""},
		{`for range n { func() { for i := range s { _ = s[i] } }() }`, "1:s", ""},
		{`for i := range s { _ = t[i] }`, "0:s", "s"},
		{`for i := range s { s = t; _ = s[i] }`, "", ""},
		{`for i := range str { _ = str[i] }`, "", ""},
		{`for i := range g { _ = g[i] }`, "", ""},
		{`_ = s[0]`, "", ""},
	} {
		_, _, info, body := bceFunc(t, c.body)
		_, hdr := inBoundsIndices(info, body)
		var loops []string
		var outer []string
		k := 0
		ast.Inspect(body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				if hdr != nil && len(hdr.loops[n.(ast.Stmt)]) > 0 {
					var names []string
					for _, v := range hdr.loops[n.(ast.Stmt)] {
						names = append(names, v.Name())
					}
					loops = append(loops, fmt.Sprintf("%d:%s", k, strings.Join(names, ",")))
				}
				k++
			}
			return true
		})
		if hdr != nil {
			for v, ok := range hdr.outer {
				if ok {
					outer = append(outer, v.Name())
				}
			}
		}
		slices.Sort(outer)
		if got := strings.Join(loops, " "); got != c.loops {
			t.Errorf("%s: loops = %q, want %q", c.body, got, c.loops)
		}
		if got := strings.Join(outer, " "); got != c.outer {
			t.Errorf("%s: outer = %q, want %q", c.body, got, c.outer)
		}
	}
}

// bceFunc type-checks body as the body of a function f with local slices
// s and t, a string str, an int n and a map ms, next to a package-level
// slice g and functions grow and pair.
func bceFunc(t *testing.T, body string) (string, *token.FileSet, *types.Info, *ast.BlockStmt) {
	t.Helper()
	return checkFunc(t, `
var g []int

func grow(s []int, n int) []int { return append(s, make([]int, n)...) }

func pair() (int, int) { return 0, 0 }
`, "s, t []int, str string, n int, ms map[int]int", body)
}

// checkFunc type-checks a package p of decls and a function f with params
// and body, and returns the source, its positions, the type information and
// the body of f.
func checkFunc(t *testing.T, decls, params, body string) (string, *token.FileSet, *types.Info, *ast.BlockStmt) {
	t.Helper()
	src := "package p\n" + decls + "\nfunc f(" + params + ") {\n\t" + body + "\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	if _, err := (&types.Config{}).Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "f" {
			return src, fset, info, fd.Body
		}
	}
	panic("no f")
}
