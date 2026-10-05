package lower

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

// inBoundsIndices finds the index expressions s[i] of a function body whose
// index is in range by construction, so that they need no bounds check:
//
//	for i := range s { ... s[i] ... }
//	for i := lo; i < len(s); i++ { ... s[i] ... }   // lo >= 0, i += c (c >= 0)
//	s := make([]T, n); for i := lo; i < n; i++ { ... s[i] ... }
//	n := len(s); for i := range n { ... s[i] ... }   // or i < n as above
//
// s is a local slice or string variable that the function never assigns or
// addresses after declaring it (a slice's header and a string never change,
// so len(s) is fixed), and i is not assigned in the body either. The bound
// n of the third form is a constant or such a local integer variable, the
// length s was made with; in the fourth, n is such a variable set to len(s).
// lo and c are non-negative when they are a constant, len(x), a range index
// or such a loop variable, or a sum or (for an int, a JS number that does not
// wrap) a product of those.
//
// An array element a[i] needs no bounds check either where i ranges over an
// array or counts up to a constant, and a is at least as long.
//
// Nor does s[x % len(s)] for a local slice or string variable s and an x
// that is non-negative as above: the remainder is in [0, len(s)), and an
// empty s panics on the division first.
func inBoundsIndices(info *types.Info, body *ast.BlockStmt) map[*ast.IndexExpr]bool {
	if body == nil {
		return nil
	}
	// Variables assigned or addressed after their declaration, with the
	// statements assigning them (a loop's post statement is allowed).
	assigned := map[*types.Var][]ast.Node{}
	note := func(e ast.Expr, n ast.Node) {
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			if v, ok := info.Uses[id].(*types.Var); ok {
				assigned[v] = append(assigned[v], n)
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				note(l, n) // a definition is in Defs, not Uses
			}
		case *ast.IncDecStmt:
			note(n.X, n)
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				note(n.X, n)
			}
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				if n.Key != nil {
					note(n.Key, n)
				}
				if n.Value != nil {
					note(n.Value, n)
				}
			}
		}
		return true
	})
	varOf := func(e ast.Expr) *types.Var {
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			v, _ := info.ObjectOf(id).(*types.Var)
			return v
		}
		return nil
	}
	local := func(v *types.Var) bool {
		return v != nil && !v.IsField() && v.Parent() != nil && v.Pkg() != nil && v.Parent() != v.Pkg().Scope()
	}
	fixed := func(v *types.Var) bool {
		if !local(v) || len(assigned[v]) > 0 {
			return false
		}
		switch u := v.Type().Underlying().(type) {
		case *types.Slice:
			return true
		case *types.Basic:
			return u.Info()&types.IsString != 0
		}
		return false
	}
	nonneg := map[*types.Var]bool{} // range indices and counting-up loop variables
	var isNonneg func(e ast.Expr) bool
	isNonneg = func(e ast.Expr) bool {
		e = ast.Unparen(e)
		if tv, ok := info.Types[e]; ok && tv.Value != nil {
			return constant.Sign(tv.Value) >= 0
		}
		switch e := e.(type) {
		case *ast.Ident:
			return nonneg[varOf(e)]
		case *ast.BinaryExpr:
			switch e.Op {
			case token.ADD:
				return isNonneg(e.X) && isNonneg(e.Y)
			case token.MUL: // an int is a JS number: a product does not wrap
				b, ok := types.Unalias(info.TypeOf(e)).(*types.Basic)
				return ok && b.Kind() == types.Int && isNonneg(e.X) && isNonneg(e.Y)
			}
			return false
		case *ast.CallExpr:
			if id, ok := ast.Unparen(e.Fun).(*ast.Ident); ok {
				if b, ok := info.Uses[id].(*types.Builtin); ok {
					return b.Name() == "len" || b.Name() == "cap"
				}
			}
		}
		return false
	}
	isInt := func(v *types.Var) bool {
		b, ok := v.Type().Underlying().(*types.Basic)
		return ok && b.Info()&types.IsInteger != 0 && !isBig(v.Type())
	}
	// The lengths of the fixed slices made by s := make([]T, n): n is a
	// constant or a fixed local integer variable, so len(s) == n wherever
	// s is in scope.
	madeLen := map[*types.Var]ast.Expr{}
	// The fixed local integer variables set by n := len(s) for a fixed s.
	lenOf := map[*types.Var]*types.Var{}
	// lenArg returns s in len(s) for a fixed s.
	lenArg := func(e ast.Expr) *types.Var {
		call, ok := ast.Unparen(e).(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return nil
		}
		if id, ok := ast.Unparen(call.Fun).(*ast.Ident); !ok || info.Uses[id] != types.Universe.Lookup("len") {
			return nil
		}
		if s := varOf(call.Args[0]); fixed(s) {
			return s
		}
		return nil
	}
	made := func(lhs, rhs ast.Expr) {
		if n := varOf(lhs); local(n) && len(assigned[n]) == 0 && isInt(n) {
			if s := lenArg(rhs); s != nil {
				lenOf[n] = s
				return
			}
		}
		s := varOf(lhs)
		call, ok := ast.Unparen(rhs).(*ast.CallExpr)
		if !ok || len(call.Args) < 2 || !fixed(s) {
			return
		}
		if _, ok := under(s.Type()).(*types.Slice); !ok {
			return
		}
		if id, ok := ast.Unparen(call.Fun).(*ast.Ident); !ok || info.Uses[id] != types.Universe.Lookup("make") {
			return
		}
		n := ast.Unparen(call.Args[1])
		if tv, ok := info.Types[n]; ok && tv.Value != nil {
			madeLen[s] = n
		} else if v := varOf(n); local(v) && len(assigned[v]) == 0 && isInt(v) {
			madeLen[s] = n
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE && len(n.Lhs) == len(n.Rhs) {
				for k := range n.Lhs {
					made(n.Lhs[k], n.Rhs[k])
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for k := range n.Names {
					made(n.Names[k], n.Values[k])
				}
			}
		}
		return true
	})
	// sameLen reports whether bound, the right of i < bound, is the length
	// slice s was made with.
	sameLen := func(s *types.Var, bound ast.Expr) bool {
		n, ok := madeLen[s]
		if !ok {
			return false
		}
		if tn, ok := info.Types[n]; ok && tn.Value != nil {
			tb, ok := info.Types[bound]
			return ok && tb.Value != nil && constant.Compare(tn.Value, token.EQL, tb.Value)
		}
		return varOf(n) == varOf(bound)
	}
	out := map[*ast.IndexExpr]bool{}
	mark := func(body ast.Node, s, i *types.Var) {
		ast.Inspect(body, func(n ast.Node) bool {
			if ix, ok := n.(*ast.IndexExpr); ok && varOf(ix.X) == s && varOf(ix.Index) == i {
				out[ix] = true
			}
			return true
		})
	}
	// markArrays marks the indices a[i] in body of arrays a (or pointers to
	// arrays) at least max+1 long, for an i that is at most max.
	markArrays := func(body ast.Node, i *types.Var, max int64) {
		ast.Inspect(body, func(n ast.Node) bool {
			if ix, ok := n.(*ast.IndexExpr); ok && varOf(ix.Index) == i {
				if a, ok := arrayType(info.TypeOf(ix.X)); ok && a.Len() > max {
					out[ix] = true
				}
			}
			return true
		})
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IndexExpr:
			// s[x % len(s)]; the loops declaring x are visited before.
			rem, ok := ast.Unparen(n.Index).(*ast.BinaryExpr)
			if !ok || rem.Op != token.REM || !isNonneg(rem.X) {
				return true
			}
			s := varOf(n.X)
			if !local(s) {
				return true
			}
			switch u := under(s.Type()).(type) {
			case *types.Slice:
			case *types.Basic:
				if u.Info()&types.IsString == 0 {
					return true
				}
			default:
				return true
			}
			if call, ok := ast.Unparen(rem.Y).(*ast.CallExpr); ok && len(call.Args) == 1 && varOf(call.Args[0]) == s {
				if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && info.Uses[id] == types.Universe.Lookup("len") {
					out[n] = true
				}
			}
		case *ast.RangeStmt:
			if n.Tok != token.DEFINE || n.Key == nil {
				return true
			}
			k := varOf(n.Key)
			if k == nil || len(assigned[k]) > 0 || !isInt(k) {
				return true
			}
			switch u := under(info.TypeOf(n.X)).(type) {
			case *types.Slice, *types.Array, *types.Pointer: // indices
			case *types.Basic: // a string's indices or an integer's 0 <= k < n
				if u.Info()&(types.IsString|types.IsInteger) == 0 {
					return true
				}
			default: // map keys, iterator values
				return true
			}
			nonneg[k] = true
			if a, ok := arrayType(info.TypeOf(n.X)); ok {
				markArrays(n.Body, k, a.Len()-1)
			} else if tv := info.Types[n.X]; tv.Value != nil {
				if c, ok := constant.Int64Val(constant.ToInt(tv.Value)); ok {
					markArrays(n.Body, k, c-1)
				}
			}
			if s := varOf(n.X); fixed(s) {
				mark(n.Body, s, k)
			} else if s := lenOf[s]; s != nil { // for i := range n, n = len(s)
				mark(n.Body, s, k)
			} else if s := lenArg(n.X); s != nil { // for i := range len(s)
				mark(n.Body, s, k)
			}
		case *ast.ForStmt:
			init, ok := n.Init.(*ast.AssignStmt)
			if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
				return true
			}
			i := varOf(init.Lhs[0])
			if i == nil || !isInt(i) || !isNonneg(init.Rhs[0]) {
				return true
			}
			// The only assignment of i: i++ or i += c (c > 0) as the post
			// statement.
			if as := assigned[i]; len(as) != 1 || as[0] != n.Post {
				return true
			}
			switch p := n.Post.(type) {
			case *ast.IncDecStmt:
				if p.Tok != token.INC {
					return true
				}
			case *ast.AssignStmt:
				if p.Tok != token.ADD_ASSIGN || !isNonneg(p.Rhs[0]) {
					return true
				}
			default:
				return true
			}
			nonneg[i] = true
			cond, ok := ast.Unparen(n.Cond).(*ast.BinaryExpr)
			if !ok || cond.Op != token.LSS || varOf(cond.X) != i {
				return true
			}
			if tv := info.Types[cond.Y]; tv.Value != nil {
				if c, ok := constant.Int64Val(constant.ToInt(tv.Value)); ok {
					markArrays(n.Body, i, c-1)
				}
			}
			if s := lenOf[varOf(cond.Y)]; s != nil {
				mark(n.Body, s, i)
				return true
			}
			if varOf(cond.Y) != nil || info.Types[cond.Y].Value != nil {
				for s := range madeLen {
					if sameLen(s, cond.Y) {
						mark(n.Body, s, i)
					}
				}
				return true
			}
			if s := lenArg(cond.Y); s != nil {
				mark(n.Body, s, i)
			}
		}
		return true
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// arrayType returns the array type of t, an array or a pointer to one.
func arrayType(t types.Type) (*types.Array, bool) {
	if p, ok := under(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	a, ok := under(t).(*types.Array)
	return a, ok
}
