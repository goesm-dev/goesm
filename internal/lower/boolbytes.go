package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// byteBools finds the local []bool variables of a function body that can be
// backed by a Uint8Array (a byte per element) instead of a JS array of
// booleans (a pointer per element): a sieve or a visited set then takes a
// quarter of the memory, and V8 reads and writes it as fast as a []byte.
// A variable qualifies when it is declared in the body and its value never
// leaves it: it is only assigned make([]bool, ...) or nil, indexed (but not
// addressed), passed to len or cap, and ranged over without a value. Its
// slices are then made with $rt.zeroByte, and every load is converted back
// to a boolean ($rt.byteBool); stores of true and false become 1 and 0.
// It returns the variables and their make calls.
func byteBools(info *types.Info, body *ast.BlockStmt) (map[*types.Var]bool, map[*ast.CallExpr]bool) {
	if body == nil {
		return nil, nil
	}
	isBoolSlice := func(t types.Type) bool {
		s, ok := types.Unalias(t).Underlying().(*types.Slice)
		if !ok {
			return false
		}
		b, ok := types.Unalias(s.Elem()).Underlying().(*types.Basic)
		return ok && b.Kind() == types.Bool
	}
	varOf := func(e ast.Expr) *types.Var {
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok {
			return nil
		}
		v, _ := info.ObjectOf(id).(*types.Var)
		if v == nil || v.IsField() || !isBoolSlice(v.Type()) {
			return nil
		}
		return v
	}
	isMake := func(e ast.Expr) *ast.CallExpr {
		call, ok := ast.Unparen(e).(*ast.CallExpr)
		if !ok {
			return nil
		}
		if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok {
			if b, ok := info.Uses[id].(*types.Builtin); ok && b.Name() == "make" && isBoolSlice(info.TypeOf(call)) {
				return call
			}
		}
		return nil
	}
	isNil := func(e ast.Expr) bool {
		tv, ok := info.Types[e]
		return ok && tv.IsNil()
	}
	cands := map[*types.Var]bool{}
	bad := map[*types.Var]bool{}
	allowed := map[*ast.Ident]bool{}
	makes := map[*ast.CallExpr]*types.Var{}
	assign := func(lhs, rhs ast.Expr) {
		v := varOf(lhs)
		if v == nil {
			return
		}
		allowed[lhs.(*ast.Ident)] = true
		switch {
		case rhs == nil || isNil(rhs):
		case isMake(rhs) != nil:
			makes[isMake(rhs)] = v
		default:
			bad[v] = true
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if (n.Tok == token.DEFINE || n.Tok == token.ASSIGN) && len(n.Lhs) == len(n.Rhs) {
				for i, l := range n.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						if v := varOf(id); v != nil && n.Tok == token.DEFINE && info.Defs[id] == v {
							cands[v] = true
						}
					}
					assign(l, n.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			if len(n.Values) == 0 || len(n.Values) == len(n.Names) {
				for i, id := range n.Names {
					if v := varOf(id); v != nil {
						cands[v] = true
						var rhs ast.Expr
						if len(n.Values) > 0 {
							rhs = n.Values[i]
						}
						assign(id, rhs)
					}
				}
			}
		case *ast.UnaryExpr:
			if ix, ok := ast.Unparen(n.X).(*ast.IndexExpr); ok && n.Op == token.AND {
				if v := varOf(ix.X); v != nil {
					bad[v] = true
				}
			}
		case *ast.IndexExpr:
			if id, ok := ast.Unparen(n.X).(*ast.Ident); ok && varOf(id) != nil {
				allowed[id] = true
			}
		case *ast.CallExpr:
			if id, ok := ast.Unparen(n.Fun).(*ast.Ident); ok && len(n.Args) == 1 {
				if b, ok := info.Uses[id].(*types.Builtin); ok && (b.Name() == "len" || b.Name() == "cap") {
					if a, ok := ast.Unparen(n.Args[0]).(*ast.Ident); ok && varOf(a) != nil {
						allowed[a] = true
					}
				}
			}
		case *ast.RangeStmt:
			if id, ok := ast.Unparen(n.X).(*ast.Ident); ok && n.Value == nil && varOf(id) != nil {
				allowed[id] = true
			}
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && !allowed[id] {
			if v, ok := info.ObjectOf(id).(*types.Var); ok && cands[v] {
				bad[v] = true
			}
		}
		return true
	})
	vars := map[*types.Var]bool{}
	for v := range cands {
		if !bad[v] {
			vars[v] = true
		}
	}
	calls := map[*ast.CallExpr]bool{}
	for c, v := range makes {
		if vars[v] {
			calls[c] = true
		}
	}
	if len(vars) == 0 {
		return nil, nil
	}
	return vars, calls
}
