package lower

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

// scalarRangeVars finds the struct-typed value variables of range loops in a
// function body that are only read field by field:
//
//	for _, d := range deps { if d.src.version() != d.version { ... } }
//
// Such a variable needs no copy of the element ($clone, an allocation per
// iteration): the loop loads the fields it reads into locals instead, which
// is the same copy, made as the iteration starts, of what the body sees. It
// maps each variable to the indices of the fields read, in order.
//
// Every use of the variable must select a field of its own (not one promoted
// from an embedded struct) whose type is not an aggregate or a type
// parameter, and must not assign it, take its address, or call a pointer
// method on it.
func scalarRangeVars(info *types.Info, body *ast.BlockStmt) map[*types.Var][]int {
	if body == nil {
		return nil
	}
	cands := map[*types.Var]*types.Struct{}
	ast.Inspect(body, func(n ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok || r.Tok != token.DEFINE || r.Value == nil {
			return true
		}
		id, ok := r.Value.(*ast.Ident)
		if !ok || id.Name == "_" {
			return true
		}
		v, _ := info.Defs[id].(*types.Var)
		if v == nil {
			return true
		}
		switch under(info.TypeOf(r.X)).(type) {
		case *types.Slice, *types.Array, *types.Map:
		default:
			if _, ok := arrayType(info.TypeOf(r.X)); !ok {
				return true
			}
		}
		if _, ok := types.Unalias(v.Type()).(*types.TypeParam); ok {
			return true
		}
		if st, ok := v.Type().Underlying().(*types.Struct); ok {
			cands[v] = st
		}
		return true
	})
	if len(cands) == 0 {
		return nil
	}
	fields := map[*types.Var][]int{}
	bad := map[*types.Var]bool{}
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		v, _ := info.Uses[id].(*types.Var)
		if cands[v] == nil || bad[v] {
			return true
		}
		if i, ok := fieldRead(info, stack); ok {
			if !slices.Contains(fields[v], i) {
				fields[v] = append(fields[v], i)
			}
		} else {
			bad[v] = true
		}
		return true
	})
	out := map[*types.Var][]int{}
	for v := range cands {
		if !bad[v] {
			f := fields[v]
			slices.Sort(f)
			out[v] = f
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fieldRead reports whether the identifier at the top of stack (its
// ancestors below it) is the operand of a read of a direct field whose
// value is not an aggregate, and returns the field's index.
func fieldRead(info *types.Info, stack []ast.Node) (int, bool) {
	n := len(stack)
	if n < 2 {
		return 0, false
	}
	sel, ok := stack[n-2].(*ast.SelectorExpr)
	if !ok || sel.X != stack[n-1] {
		return 0, false
	}
	s, ok := info.Selections[sel]
	if !ok || s.Kind() != types.FieldVal || len(s.Index()) != 1 {
		return 0, false
	}
	ft := s.Obj().Type()
	if _, ok := types.Unalias(ft).(*types.TypeParam); ok {
		return 0, false
	}
	switch ft.Underlying().(type) {
	case *types.Struct, *types.Array:
		return 0, false
	}
	if n >= 3 {
		switch p := stack[n-3].(type) {
		case *ast.AssignStmt:
			if slices.Contains(p.Lhs, ast.Expr(sel)) {
				return 0, false
			}
		case *ast.IncDecStmt:
			return 0, false
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				return 0, false
			}
		case *ast.RangeStmt:
			if p.Tok == token.ASSIGN && (p.Key == sel || p.Value == sel) {
				return 0, false
			}
		case *ast.SelectorExpr: // d.f.M with a pointer method M takes &d.f
			if ms, ok := info.Selections[p]; ok && ms.Kind() == types.MethodVal {
				if fn, ok := ms.Obj().(*types.Func); ok && isPtrRecv(fn) {
					if _, ptr := ft.Underlying().(*types.Pointer); !ptr {
						return 0, false
					}
				}
			}
		case *ast.ParenExpr:
			return 0, false // (d.f) could be any of the above
		}
	}
	return s.Index()[0], true
}
