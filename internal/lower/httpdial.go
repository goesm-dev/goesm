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
// net/http.Transport (HTTPDialers). A field is told by its object, which a
// type defined as http.Transport, or embedding it, shares.
func storesDialer(info *types.Info, f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.KeyValueExpr:
			if k, ok := n.Key.(*ast.Ident); ok && isDialVar(info.Uses[k]) && !hostDialer(info, n.Value) {
				found = true
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
	return s != nil && s.Kind() == types.FieldVal && isDialVar(s.Obj())
}

// isDialVar reports whether obj is one of net/http.Transport's dialer
// fields.
func isDialVar(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	if !ok || !v.IsField() || v.Pkg() == nil || v.Pkg().Path() != "net/http" || !transportDialFields[v.Name()] {
		return false
	}
	tn, ok := v.Pkg().Scope().Lookup("Transport").(*types.TypeName)
	if !ok {
		return false
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i) == v {
			return true
		}
	}
	return false
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
