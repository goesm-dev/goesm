package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Function variables with known values. A call through a function value is
// resolved by signature over the whole program (dynSigBlocks), so it may
// block as soon as any function of that signature does. A variable of
// function type that is only ever assigned function literals, declared
// functions or nil, and whose address is never taken, can only hold those:
// a call of it calls one of them, and blocks only if one of them does.
//
// This keeps, for example, a component's update := func() {...} called from
// its event handlers synchronous in a program where some other func() value
// blocks.

// findFuncVars sets p.funcVars.
func (p *Program) findFuncVars() {
	vals := map[*types.Var][]any{}
	bad := map[*types.Var]bool{}
	defined := map[*types.Var]bool{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		varOf := func(e ast.Expr) *types.Var {
			switch e := unparen(e).(type) {
			case *ast.Ident:
				if v, ok := info.Defs[e].(*types.Var); ok {
					return v
				}
				v, _ := info.Uses[e].(*types.Var)
				return v
			case *ast.SelectorExpr:
				if _, ok := info.Selections[e]; !ok {
					v, _ := info.Uses[e.Sel].(*types.Var)
					return v
				}
			}
			return nil
		}
		// value returns the function rhs evaluates to (nil for nil), or
		// false if it is not a function literal, a declared function or nil.
		value := func(rhs ast.Expr) (any, bool) {
			var id *ast.Ident
			switch e := unparen(rhs).(type) {
			case *ast.FuncLit:
				return e, true
			case *ast.Ident:
				id = e
			case *ast.SelectorExpr:
				if _, ok := info.Selections[e]; ok {
					return nil, false // a method value
				}
				id = e.Sel
			default:
				return nil, false
			}
			switch obj := info.Uses[id].(type) {
			case *types.Nil:
				return nil, true
			case *types.Func:
				if _, inst := info.Instances[id]; inst || obj.Signature().TypeParams().Len() > 0 {
					return nil, false
				}
				return obj.Origin(), true
			}
			return nil, false
		}
		assign := func(l, r ast.Expr) {
			v := varOf(l)
			if v == nil {
				return
			}
			if r == nil {
				bad[v] = true
				return
			}
			fn, ok := value(r)
			switch {
			case !ok:
				bad[v] = true
			case fn != nil:
				vals[v] = append(vals[v], fn)
			}
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						if v, ok := info.Defs[name].(*types.Var); ok {
							defined[v] = true
						}
						switch {
						case len(n.Values) == 0:
						case len(n.Values) == len(n.Names):
							assign(name, n.Values[i])
						default:
							assign(name, nil)
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						if n.Tok == token.DEFINE {
							if id, ok := l.(*ast.Ident); ok {
								if v, ok := info.Defs[id].(*types.Var); ok {
									defined[v] = true
								}
							}
						}
						if len(n.Lhs) == len(n.Rhs) {
							assign(l, n.Rhs[i])
						} else {
							assign(l, nil)
						}
					}
				case *ast.RangeStmt:
					if n.Key != nil {
						assign(n.Key, nil)
					}
					if n.Value != nil {
						assign(n.Value, nil)
					}
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						if v := varOf(n.X); v != nil {
							bad[v] = true
						}
					}
				}
				return true
			})
		}
	}
	p.funcVars = map[*types.Var][]any{}
	for v := range defined {
		if _, isFunc := v.Type().Underlying().(*types.Signature); !isFunc || bad[v] || v.IsField() {
			continue
		}
		p.funcVars[v] = vals[v]
	}
}
