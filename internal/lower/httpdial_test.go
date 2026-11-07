package lower

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// TestStoresDialer checks which stores into a Transport's dialer fields
// keep the Transport's own connections (HTTPDialers).
func TestStoresDialer(t *testing.T) {
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	for _, c := range []struct {
		body string
		want bool
	}{
		{`_ = &http.Transport{}`, false},
		{`_ = &http.Transport{DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, Dial: net.Dial}`, false},
		{`d := &net.Dialer{}; t := &http.Transport{}; t.DialContext = d.DialContext; t.Dial = nil`, false},
		{`_ = &http.Transport{DialContext: dial}`, true},
		{`_ = &http.Transport{DialTLSContext: func(context.Context, string, string) (net.Conn, error) { return nil, nil }}`, true},
		{`t := http.DefaultTransport.(*http.Transport).Clone(); t.DialContext = dial`, true},
		// A type defined as http.Transport shares its fields.
		{`type T http.Transport; _ = (*http.Transport)(&T{DialContext: dial})`, true},
		{`type T http.Transport; var x T; x.DialContext = dial; _ = (*http.Transport)(&x)`, true},
		{`type E struct{ http.Transport }; var e E; e.DialContext = dial`, true},
		{`t := &http.Transport{}; p := &t.DialContext; _ = p`, true},
		{`t := &http.Transport{}; t.DialContext, t.Dial = nil, nil`, false},
	} {
		src := `package p

import (
	"context"
	"net"
	"net/http"
	"time"
)

var _ = time.Second

func dial(ctx context.Context, network, addr string) (net.Conn, error) { return nil, nil }

func f() {
	` + c.body + `
}
`
		f, err := parser.ParseFile(fset, "p.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
		if _, err := (&types.Config{Importer: imp}).Check("p", fset, []*ast.File{f}, info); err != nil {
			t.Fatalf("%s: %v", c.body, err)
		}
		if got := storesDialer(info, f); got != c.want {
			t.Errorf("%s: storesDialer = %v, want %v", c.body, got, c.want)
		}
	}
}
