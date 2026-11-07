package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Function variables with known values. A call through a function value is
// resolved by signature over the whole program (dynSigBlocks), so it may
// block as soon as any function of that signature does. A variable of
// function type that can only hold known functions is resolved to those
// instead: a call of it calls one of them, and blocks only if one of them
// does. Such a variable is
//
//   - a local or package variable,
//   - a field of function type of a named struct type (sync.Pool's New,
//     which Pool.Get calls), or
//   - a parameter of function type of a function declared without a
//     receiver that is only ever called directly: never used as a value,
//     linked with //go:linkname or called from JavaScript as an export,
//
// whose address is never taken, and every value of which, assigned, stored
// in a composite literal or passed as an argument, is a function literal, a
// declared function, nil, or another such variable, whose values it then
// holds too.
//
// This keeps, for example, a component's update := func() {...} called from
// its event handlers synchronous in a program where some other func() value
// blocks, and Pool.Get synchronous unless some pool's New blocks.

// findFuncVars sets p.funcVars.
func (p *Program) findFuncVars() {
	vals := map[*types.Var][]any{}
	deps := map[*types.Var][]*types.Var{}
	bad := map[*types.Var]bool{}
	cand := map[*types.Var]bool{}
	fieldStores := map[*ast.Ident]*types.Var{}
	isFunc := func(v *types.Var) bool {
		_, ok := v.Type().Underlying().(*types.Signature)
		return ok
	}
	// badFields marks the function fields of the struct t, or t points to,
	// unknown: a conversion gives a value of t fields that were stored as
	// fields of another type.
	badFields := func(t types.Type) {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		if st, ok := t.Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				bad[st.Field(i).Origin()] = true
			}
		}
	}

	// The uses of declared functions that call them, the functions linked
	// with //go:linkname, and the fields of named struct types.
	called := map[*ast.Ident]bool{}
	linked := map[string]bool{}
	linkedLocal := map[types.Object]bool{}
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range linkDirectives(f) {
				if d.target != "" {
					linked[d.target] = true
				}
				if obj := pkg.Types.Scope().Lookup(d.local); obj != nil {
					linkedLocal[obj] = true
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					fun := unparen(c.Fun)
					switch x := fun.(type) {
					case *ast.IndexExpr:
						fun = x.X
					case *ast.IndexListExpr:
						fun = x.X
					}
					if id := identOf(fun); id != nil {
						called[id] = true
					}
				}
				return true
			})
		}
		for _, obj := range pkg.TypesInfo.Defs {
			tn, ok := obj.(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			if st, ok := tn.Type().Underlying().(*types.Struct); ok {
				if _, named := tn.Type().(*types.Named); named {
					for i := 0; i < st.NumFields(); i++ {
						if v := st.Field(i); isFunc(v) {
							cand[v] = true
						}
					}
				}
			}
		}
	}
	asValue := map[*types.Func]bool{}
	for _, pkg := range p.Pkgs {
		for id, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && !called[id] {
				asValue[fn.Origin()] = true
			}
		}
	}
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv != nil {
					continue
				}
				fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				unknown := asValue[fn] || linkedLocal[fn] || linked[linkSymbol(fn)] || p.Entries[pkg.PkgPath] && fn.Exported()
				params := fn.Signature().Params()
				for i := 0; i < params.Len(); i++ {
					if v := params.At(i); isFunc(v) {
						cand[v] = true
						if unknown || fn.Signature().Variadic() && i == params.Len()-1 {
							bad[v] = true
						}
					}
				}
			}
		}
	}

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
				sel, ok := info.Selections[e]
				if !ok {
					v, _ := info.Uses[e.Sel].(*types.Var)
					return v
				}
				if sel.Kind() == types.FieldVal {
					return sel.Obj().(*types.Var).Origin()
				}
			}
			return nil
		}
		// store records rhs as a value of v: a function literal, a declared
		// function or nil, or a variable whose values flow into v. Anything
		// else makes v unknown.
		store := func(v *types.Var, rhs ast.Expr) {
			if rhs == nil {
				bad[v] = true
				return
			}
			var id *ast.Ident
			switch e := unparen(rhs).(type) {
			case *ast.FuncLit:
				vals[v] = append(vals[v], e)
				return
			case *ast.Ident:
				id = e
			case *ast.SelectorExpr:
				if sel, ok := info.Selections[e]; ok {
					if sel.Kind() == types.FieldVal {
						deps[v] = append(deps[v], sel.Obj().(*types.Var).Origin())
					} else {
						bad[v] = true // a method value or expression
					}
					return
				}
				id = e.Sel
			default:
				bad[v] = true
				return
			}
			switch obj := info.Uses[id].(type) {
			case *types.Nil:
				return
			case *types.Func:
				if _, inst := info.Instances[id]; inst || obj.Signature().TypeParams().Len() > 0 {
					bad[v] = true
					return
				}
				vals[v] = append(vals[v], obj.Origin())
				if v.IsField() {
					fieldStores[id] = v
				}
				return
			case *types.Var:
				deps[v] = append(deps[v], obj)
				return
			}
			bad[v] = true
		}
		assign := func(l, r ast.Expr) {
			if v := varOf(l); v != nil && isFunc(v) {
				store(v, r)
			}
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						v, ok := info.Defs[name].(*types.Var)
						if !ok || !isFunc(v) {
							continue
						}
						cand[v] = true
						switch {
						case len(n.Values) == 0:
						case len(n.Values) == len(n.Names):
							store(v, n.Values[i])
						default:
							bad[v] = true
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						if n.Tok == token.DEFINE {
							if id, ok := l.(*ast.Ident); ok {
								if v, ok := info.Defs[id].(*types.Var); ok && isFunc(v) {
									cand[v] = true
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
				case *ast.CompositeLit:
					t := info.TypeOf(n)
					if t == nil {
						return true
					}
					st, ok := t.Underlying().(*types.Struct)
					if !ok {
						return true
					}
					for i, elt := range n.Elts {
						var field *types.Var
						val := elt
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if id, ok := kv.Key.(*ast.Ident); ok {
								field, _ = info.Uses[id].(*types.Var)
							}
							val = kv.Value
						} else if i < st.NumFields() {
							field = st.Field(i)
						}
						if field != nil && isFunc(field) {
							store(field.Origin(), val)
						}
					}
				case *ast.CallExpr:
					if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
						badFields(tv.Type)
						return true
					}
					fun := unparen(n.Fun)
					switch x := fun.(type) {
					case *ast.IndexExpr:
						fun = x.X
					case *ast.IndexListExpr:
						fun = x.X
					}
					id := identOf(fun)
					if id == nil {
						return true
					}
					fn, ok := info.Uses[id].(*types.Func)
					if !ok || fn.Signature().Recv() != nil {
						return true
					}
					params := fn.Origin().Signature().Params()
					if n.Ellipsis.IsValid() || len(n.Args) != params.Len() {
						// f(g()) with several results, or a variadic call
						for i := 0; i < params.Len(); i++ {
							bad[params.At(i)] = true
						}
						return true
					}
					for i, a := range n.Args {
						if v := params.At(i); isFunc(v) {
							store(v, a)
						}
					}
				}
				return true
			})
		}
	}

	// A variable some value of which is not known is not known either.
	for changed := true; changed; {
		changed = false
		for v := range cand {
			if bad[v] {
				continue
			}
			for _, d := range deps[v] {
				if bad[d] || !cand[d] {
					bad[v] = true
					changed = true
					break
				}
			}
		}
	}
	// The values of a variable flow into the variables depending on it.
	resolved := map[*types.Var]map[any]bool{}
	for v := range cand {
		if !bad[v] {
			set := map[any]bool{}
			for _, f := range vals[v] {
				set[f] = true
			}
			resolved[v] = set
		}
	}
	for changed := true; changed; {
		changed = false
		for v, set := range resolved {
			for _, d := range deps[v] {
				for f := range resolved[d] {
					if !set[f] {
						set[f] = true
						changed = true
					}
				}
			}
		}
	}
	p.funcFieldStores = map[*ast.Ident]*types.Var{}
	for id, v := range fieldStores {
		if resolved[v] != nil {
			p.funcFieldStores[id] = v
		}
	}
	p.funcVars = map[*types.Var][]any{}
	for v, set := range resolved {
		var fns []any
		seen := map[any]bool{}
		for _, f := range vals[v] {
			if !seen[f] {
				seen[f] = true
				fns = append(fns, f)
			}
		}
		// Values from deps, in a stable order: by the order the variables
		// they came from were stored.
		var walk func(d *types.Var, visited map[*types.Var]bool)
		walk = func(d *types.Var, visited map[*types.Var]bool) {
			if visited[d] {
				return
			}
			visited[d] = true
			for _, f := range vals[d] {
				if set[f] && !seen[f] {
					seen[f] = true
					fns = append(fns, f)
				}
			}
			for _, dd := range deps[d] {
				walk(dd, visited)
			}
		}
		visited := map[*types.Var]bool{v: true}
		for _, d := range deps[v] {
			walk(d, visited)
		}
		p.funcVars[v] = fns
	}
}
