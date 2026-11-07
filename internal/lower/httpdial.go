package lower

import (
	"go/ast"
	"go/types"
)

// HTTP clients over fetch. net/http's Transport sends a request with fetch
// on a JavaScript host unless it has dialers of its own (the patch of
// roundtrip_js.go), and its own connections, with TLS, HTTP/2, X.509 and
// the elliptic curves they keep, are left out (leftOut in emit.go, reach)
// when the program gives no Transport a dialer: a program calling http.Get
// was 2.6 MB minified, a request with fetch takes a few KB.

// transportRoundTrip is the Transport's own round trip, which dials.
const transportRoundTrip = "(*net/http.Transport).roundTrip"

// transportDialFields are the fields of net/http.Transport holding dialers.
var transportDialFields = map[string]bool{"Dial": true, "DialContext": true, "DialTLS": true, "DialTLSContext": true}

// HTTPDialers reports whether a package of the program other than net/http
// may store a dialer of its own in a net/http.Transport: a value other than
// nil, net.Dial or a method value of a *net.Dialer, which dial only the host
// network net/http's Transport never reaches on a JavaScript host, in one of
// its Dial, DialContext, DialTLS or DialTLSContext fields, or takes the
// address of one. Such a Transport connects with its own dialer, and keeps
// the code of its connections.
func (p *Program) HTTPDialers() bool {
	p.httpDialersOnce.Do(func() {
		for _, pkg := range p.Pkgs {
			if pkg.PkgPath == "net/http" {
				continue
			}
			for _, f := range pkg.Syntax {
				if storesDialer(pkg.TypesInfo, f) {
					p.httpDialers = true
					return
				}
			}
		}
	})
	return p.httpDialers
}

// storesDialer reports whether f stores a dialer of its own in a
// net/http.Transport (HTTPDialers).
func storesDialer(info *types.Info, f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if !isTransport(info.TypeOf(n)) {
				break
			}
			for _, e := range n.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok && transportDialFields[k.Name] && !hostDialer(info, kv.Value) {
					found = true
				}
			}
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				if !isDialField(info, l) {
					continue
				}
				if len(n.Lhs) != len(n.Rhs) || !hostDialer(info, n.Rhs[i]) {
					found = true
				}
			}
		case *ast.UnaryExpr:
			if isDialField(info, n.X) {
				found = true // &t.DialContext
			}
		}
		return !found
	})
	return found
}

// isDialField reports whether e selects a dialer field of a
// net/http.Transport.
func isDialField(info *types.Info, e ast.Expr) bool {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	s := info.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal || !transportDialFields[sel.Sel.Name] {
		return false
	}
	v, ok := s.Obj().(*types.Var)
	return ok && v.Pkg() != nil && v.Pkg().Path() == "net/http" && isTransport(fieldOwner(s))
}

// fieldOwner returns the struct type declaring the field s selects.
func fieldOwner(s *types.Selection) types.Type {
	t := s.Recv()
	idx := s.Index()
	for _, i := range idx[:len(idx)-1] {
		t, _ = derefType(t)
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return nil
		}
		t = st.Field(i).Type()
	}
	t, _ = derefType(t)
	return t
}

// isTransport reports whether t is net/http.Transport.
func isTransport(t types.Type) bool {
	if t == nil {
		return false
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "net/http" && n.Obj().Name() == "Transport"
}

// hostDialer reports whether e is nil, net.Dial or a method value of a
// *net.Dialer.
func hostDialer(info *types.Info, e ast.Expr) bool {
	e = ast.Unparen(e)
	if tv, ok := info.Types[e]; ok && tv.IsNil() {
		return true
	}
	switch e := e.(type) {
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil {
			if s.Kind() != types.MethodVal {
				return false
			}
			fn, ok := s.Obj().(*types.Func)
			return ok && (fn.FullName() == "(*net.Dialer).Dial" || fn.FullName() == "(*net.Dialer).DialContext")
		}
		fn, ok := info.Uses[e.Sel].(*types.Func)
		return ok && fn.FullName() == "net.Dial"
	}
	return false
}
