package lower

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// Interface variables with known dynamic types. A call of a method through
// an interface value may reach the method of every type the program stores
// in interface values (ifaceCallBlocks), so it blocks as soon as one of
// those methods does: one io.Reader whose Read waits made every Read
// through an io.Reader block, crypto/rand.Read's of its package variable
// Reader among them. A variable of interface type that can only hold values
// of known types is resolved to those instead: a call through it calls the
// method of one of them, and blocks only if one of those does. Such a
// variable is
//
//   - a local or package variable,
//   - an unexported field of a named struct type (encoding/json's
//     decoder's reader), or a field of an unnamed struct type, which is one
//     variable for all identical unnamed struct types (struct{ io.Reader }
//     hides a reader's other methods), unless it is exported and
//     reflection can set it,
//   - a parameter of a declared function or method that is only ever
//     called directly: never used as a value, linked with //go:linkname,
//     called from JavaScript, or, for a method, called through an interface
//     that has a method of its name or by reflection, or
//   - a result of a declared function with a body,
//
// whose address is never taken, and every value of which, assigned, stored
// in a composite literal, passed as an argument or returned, is nil, of a
// type that is not an interface or a type parameter, or another such
// variable (or the result of a call of a declared function that is one),
// whose values it then holds too. A conversion between struct types or
// pointers to them, and an assignment between a named and an unnamed struct
// type with the same fields, make the fields of both unknown.

// findIfaceVars sets p.ifaceVars.
func (p *Program) findIfaceVars() {
	vals := map[*types.Var][]types.Type{}
	deps := map[*types.Var][]*types.Var{}
	bad := map[*types.Var]bool{}
	cand := map[*types.Var]bool{}
	var canon typeutil.Map
	canonical := func(t types.Type) types.Type {
		if c := canon.At(t); c != nil {
			return c.(types.Type)
		}
		canon.Set(t, t)
		return t
	}
	isIfaceVar := func(v *types.Var) bool {
		if _, ok := types.Unalias(v.Type()).(*types.TypeParam); ok {
			return false
		}
		iface, ok := v.Type().Underlying().(*types.Interface)
		return ok && iface.NumMethods() > 0
	}
	hasIfaceField := func(st *types.Struct) bool {
		for i := 0; i < st.NumFields(); i++ {
			if isIfaceVar(st.Field(i)) {
				return true
			}
		}
		return false
	}
	// The fields of identical unnamed struct types hold the same values:
	// a value of one is a value of the other. They are one variable, the
	// field of the first of them (p.unnamedFields).
	p.unnamedFields = map[*types.Var]*types.Var{}
	var canonStruct typeutil.Map
	reflectSets := p.reflectSets()
	settable := map[*types.Struct]bool{}
	// isSettable reports whether reflection can set the fields of a value
	// of the unnamed struct type st: whether it reaches one through a
	// pointer or a slice.
	isSettable := func(st *types.Struct) bool {
		if r, ok := settable[st]; ok {
			return r
		}
		// contains reports whether a value of type t holds a value of st
		// (Go has no recursive value types).
		memo := map[types.Type]bool{}
		var contains func(t types.Type) bool
		contains = func(t types.Type) bool {
			if r, ok := memo[t]; ok {
				return r
			}
			r := false
			switch u := t.Underlying().(type) {
			case *types.Struct:
				// A named type's fields are its own.
				_, unnamed := types.Unalias(t).(*types.Struct)
				r = unnamed && types.Identical(u, st)
				for i := 0; i < u.NumFields() && !r; i++ {
					r = contains(u.Field(i).Type())
				}
			case *types.Array:
				r = contains(u.Elem())
			}
			memo[t] = r
			return r
		}
		r := false
		if reflectSets {
			p.ifaceTypes.Iterate(func(t types.Type, _ any) {
				switch u := t.Underlying().(type) {
				case *types.Pointer:
					r = r || contains(u.Elem())
				case *types.Slice:
					r = r || contains(u.Elem())
				}
			})
		}
		settable[st] = r
		return r
	}
	// fieldOf returns the variable of field i of the struct type t.
	fieldOf := func(t types.Type, i int) *types.Var {
		t = types.Unalias(t)
		st := t.Underlying().(*types.Struct)
		if _, named := t.(*types.Named); named {
			return st.Field(i).Origin()
		}
		if c, ok := p.unnamedFields[st.Field(i)]; ok {
			return c
		}
		cs := st
		if c := canonStruct.At(st); c != nil {
			cs = c.(*types.Struct)
		} else {
			canonStruct.Set(st, st)
		}
		for j := 0; j < st.NumFields(); j++ {
			f := cs.Field(j)
			p.unnamedFields[st.Field(j)] = f
			if isIfaceVar(f) && !cand[f] {
				cand[f] = true
				if f.Exported() && isSettable(cs) {
					bad[f] = true
				}
			}
		}
		return cs.Field(i)
	}
	deref := func(t types.Type) types.Type {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			return ptr.Elem()
		}
		return t
	}
	// selField returns the variable of the field sel selects.
	selField := func(sel *types.Selection) *types.Var {
		t := sel.Recv()
		idx := sel.Index()
		for _, k := range idx[:len(idx)-1] {
			t = deref(t).Underlying().(*types.Struct).Field(k).Type()
		}
		return fieldOf(deref(t), idx[len(idx)-1])
	}
	badFields := func(t types.Type) {
		t = deref(t)
		if st, ok := t.Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				bad[fieldOf(t, i)] = true
			}
		}
	}

	// The uses of declared functions that call them, the symbols linked
	// with //go:linkname, the unexported fields of named struct types, and
	// the method names of the program's interfaces.
	called := map[*ast.Ident]bool{}
	linked := map[string]bool{}
	ifaceNames := map[string][]*types.Signature{} // by name, or path.name when unexported
	methodKey := func(pkg *types.Package, name string) string {
		if token.IsExported(name) || pkg == nil {
			return name
		}
		return pkg.Path() + "." + name
	}
	seenIface := map[*types.Interface]bool{}
	addIface := func(t types.Type) {
		if t == nil {
			return
		}
		iface, ok := t.Underlying().(*types.Interface)
		if !ok || seenIface[iface] {
			return
		}
		seenIface[iface] = true
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			k := methodKey(m.Pkg(), m.Name())
			ifaceNames[k] = append(ifaceNames[k], m.Signature())
		}
	}
	reflectCalls := false
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range linkDirectives(f) {
				if d.target != "" {
					linked[d.target] = true
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
		for _, tv := range pkg.TypesInfo.Types {
			addIface(tv.Type)
		}
		for _, obj := range pkg.TypesInfo.Defs {
			if obj != nil {
				addIface(obj.Type())
			}
		}
		// Reflection calls the exported methods it looks up (reach.go).
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && pkg.PkgPath != "reflect" && (reflectEnum[fn.FullName()] || reflectLike(fn)) && fn.Name() != "NumMethod" {
				reflectCalls = true
			}
		}
		for _, obj := range pkg.TypesInfo.Defs {
			tn, ok := obj.(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			if st, ok := tn.Type().Underlying().(*types.Struct); ok {
				if _, named := tn.Type().(*types.Named); named {
					for i := 0; i < st.NumFields(); i++ {
						if v := st.Field(i); isIfaceVar(v) {
							cand[v] = true
							if v.Exported() {
								bad[v] = true
							}
						}
					}
				}
			}
		}
	}
	// A value of an unnamed struct type is assignable to a named struct
	// type of the same underlying type, and the other way around: the
	// fields of either then hold the other's values without a store to
	// them. The assignments are those findConversions finds.
	conv := func(src, dst types.Type) {
		if src == nil || dst == nil || types.Identical(src, dst) {
			return
		}
		ss, ok := src.Underlying().(*types.Struct)
		if !ok || !hasIfaceField(ss) {
			return
		}
		if ds, ok := dst.Underlying().(*types.Struct); ok && types.Identical(ss, ds) {
			badFields(src)
			badFields(dst)
		}
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body != nil {
						if fn, ok := info.Defs[d.Name].(*types.Func); ok {
							p.findConversions(info, d.Body, fn.Signature(), conv)
						}
					}
				case *ast.GenDecl:
					p.findConversions(info, d, nil, conv)
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
	symbol := func(obj types.Object) string {
		if obj.Pkg().Name() == "main" {
			return "main." + obj.Name()
		}
		return obj.Pkg().Path() + "." + obj.Name()
	}
	// The functions that //go:linkname pulls call.
	provided := map[*types.Func]bool{}
	for _, fn := range p.linkTargets {
		provided[fn] = true
	}
	// ifaceCallable reports whether some interface of the program has a
	// method that may be the method fn: of its name and signature, or of
	// its name for a method of a generic type.
	ifaceCallable := func(fn *types.Func) bool {
		sig := fn.Signature()
		for _, m := range ifaceNames[methodKey(fn.Pkg(), fn.Name())] {
			if sig.RecvTypeParams().Len() > 0 || hasTypeParam(m) ||
				types.Identical(dropRecv(sig), dropRecv(m)) {
				return true
			}
		}
		return false
	}
	// JavaScript calls these with its own arguments (entryABICalls).
	jsCalls := p.entryABICalls()
	// The parameters and results of the declared functions, and the
	// return statements that store the results.
	type ret struct {
		info    *types.Info
		res     *types.Var
		results []ast.Expr
		i, n    int
	}
	var rets []ret
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				unknown := asValue[fn] || provided[fn] || fd.Body == nil || jsCalls[fn]
				if fd.Recv == nil {
					unknown = unknown || p.Entries[pkg.PkgPath] && fn.Exported()
				} else {
					unknown = unknown || ifaceCallable(fn) ||
						fn.Exported() && (reflectCalls || p.Entries[pkg.PkgPath])
				}
				sig := fn.Signature()
				params := sig.Params()
				for i := 0; i < params.Len(); i++ {
					if v := params.At(i); isIfaceVar(v) {
						cand[v] = true
						if unknown {
							bad[v] = true
						}
					}
				}
				if fd.Body == nil {
					continue
				}
				results := sig.Results()
				has := false
				for i := 0; i < results.Len(); i++ {
					if v := results.At(i); isIfaceVar(v) {
						cand[v] = true
						has = true
					}
				}
				if !has {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.FuncLit:
						return false
					case *ast.ReturnStmt:
						for i := 0; i < results.Len(); i++ {
							if v := results.At(i); isIfaceVar(v) && len(n.Results) > 0 {
								rets = append(rets, ret{pkg.TypesInfo, v, n.Results, i, results.Len()})
							}
						}
					}
					return true
				})
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
					return selField(sel)
				}
			}
			return nil
		}
		// store records rhs as a value of v: nil, a value of a type that
		// is not an interface, or a variable whose values flow into v.
		// Anything else makes v unknown.
		var store func(v *types.Var, rhs ast.Expr)
		store = func(v *types.Var, rhs ast.Expr) {
			if rhs == nil {
				bad[v] = true
				return
			}
			e := unparen(rhs)
			if id, ok := e.(*ast.Ident); ok {
				if _, ok := info.Uses[id].(*types.Nil); ok {
					return
				}
			}
			t := info.TypeOf(e)
			if t == nil {
				bad[v] = true
				return
			}
			if b, ok := t.(*types.Basic); ok && b.Info()&types.IsUntyped != 0 {
				t = types.Default(t)
			}
			if _, ok := types.Unalias(t).(*types.TypeParam); ok {
				bad[v] = true
				return
			}
			if !types.IsInterface(t) {
				vals[v] = append(vals[v], canonical(t))
				return
			}
			if call, ok := e.(*ast.CallExpr); ok {
				if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
					// A conversion to an interface type.
					if len(call.Args) == 1 {
						store(v, call.Args[0])
					} else {
						bad[v] = true
					}
					return
				}
				if r := resultVar(info, call, 0, 1); r != nil {
					deps[v] = append(deps[v], r)
				} else {
					bad[v] = true
				}
				return
			}
			if w := varOf(e); w != nil {
				deps[v] = append(deps[v], w)
				return
			}
			bad[v] = true
		}
		// storeResult stores result i of rhs, a call of n results, in v.
		storeResult := func(v *types.Var, rhs []ast.Expr, i, n int) {
			if len(rhs) == 1 {
				if call, ok := unparen(rhs[0]).(*ast.CallExpr); ok {
					if r := resultVar(info, call, i, n); r != nil {
						deps[v] = append(deps[v], r)
						return
					}
				}
			}
			bad[v] = true
		}
		assign := func(l, r ast.Expr) {
			if v := varOf(l); v != nil && isIfaceVar(v) {
				store(v, r)
			}
		}
		for _, r := range rets {
			switch {
			case r.info != info:
			case len(r.results) == r.n:
				store(r.res, r.results[r.i])
			default:
				storeResult(r.res, r.results, r.i, r.n)
			}
		}
		for _, f := range pkg.Syntax {
			for _, d := range linkDirectives(f) {
				if obj, ok := pkg.Types.Scope().Lookup(d.local).(*types.Var); ok {
					bad[obj] = true
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						v, ok := info.Defs[name].(*types.Var)
						if !ok || !isIfaceVar(v) {
							continue
						}
						cand[v] = true
						if v.Parent() == v.Pkg().Scope() && linked[symbol(v)] {
							bad[v] = true
						}
						switch {
						case len(n.Values) == 0:
						case len(n.Values) == len(n.Names):
							store(v, n.Values[i])
						default:
							storeResult(v, n.Values, i, len(n.Names))
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						if n.Tok == token.DEFINE {
							if id, ok := l.(*ast.Ident); ok {
								if v, ok := info.Defs[id].(*types.Var); ok && isIfaceVar(v) {
									cand[v] = true
								}
							}
						}
						switch {
						case len(n.Lhs) == len(n.Rhs):
							assign(l, n.Rhs[i])
						default:
							if v := varOf(l); v != nil && isIfaceVar(v) {
								storeResult(v, n.Rhs, i, len(n.Lhs))
							}
						}
					}
				case *ast.RangeStmt:
					if n.Key != nil {
						assign(n.Key, nil)
					}
					if n.Value != nil {
						assign(n.Value, nil)
					}
				case *ast.SelectorExpr:
					// Every field read has its variable (knownTypes).
					if sel, ok := info.Selections[n]; ok && sel.Kind() == types.FieldVal {
						selField(sel)
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
					// An element of a slice, array or map of pointers
					// elides &T: its type is *T.
					if ptr, ok := t.Underlying().(*types.Pointer); ok {
						t = ptr.Elem()
					}
					st, ok := t.Underlying().(*types.Struct)
					if !ok {
						return true
					}
					for i, elt := range n.Elts {
						k := -1
						val := elt
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if id, ok := kv.Key.(*ast.Ident); ok {
								for j := 0; j < st.NumFields(); j++ {
									if st.Field(j).Name() == id.Name {
										k = j
									}
								}
							}
							val = kv.Value
						} else if i < st.NumFields() {
							k = i
						}
						if k >= 0 && isIfaceVar(st.Field(k)) {
							store(fieldOf(t, k), val)
						}
					}
				case *ast.CallExpr:
					if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
						// The fields of both types, which may share
						// memory through a pointer conversion.
						badFields(tv.Type)
						if len(n.Args) == 1 {
							if t := info.TypeOf(n.Args[0]); t != nil {
								badFields(t)
							}
						}
						return true
					}
					fun := unparen(n.Fun)
					switch x := fun.(type) {
					case *ast.IndexExpr:
						fun = x.X
					case *ast.IndexListExpr:
						fun = x.X
					}
					var fn *types.Func
					switch f := unparen(fun).(type) {
					case *ast.Ident:
						fn, _ = info.Uses[f].(*types.Func)
					case *ast.SelectorExpr:
						if sel, ok := info.Selections[f]; ok {
							switch sel.Kind() {
							case types.MethodVal:
								fn, _ = sel.Obj().(*types.Func)
							case types.MethodExpr:
								// T.M(x, ...) passes the receiver first.
								if m, ok := sel.Obj().(*types.Func); ok {
									params := m.Origin().Signature().Params()
									for i := 0; i < params.Len(); i++ {
										bad[params.At(i)] = true
									}
								}
							}
						} else {
							fn, _ = info.Uses[f.Sel].(*types.Func)
						}
					}
					if fn == nil {
						return true
					}
					sig := fn.Origin().Signature()
					if r := sig.Recv(); r != nil && isIface(r.Type()) {
						return true
					}
					params := sig.Params()
					nfixed := params.Len()
					if sig.Variadic() {
						nfixed--
					}
					if len(n.Args) == 1 && params.Len() > 1 {
						if _, ok := info.TypeOf(n.Args[0]).(*types.Tuple); ok {
							// f(g()) with several results
							for i := 0; i < params.Len(); i++ {
								if v := params.At(i); isIfaceVar(v) {
									storeResult(v, n.Args, i, params.Len())
								}
							}
							return true
						}
					}
					for i, a := range n.Args {
						if i >= nfixed {
							break
						}
						if v := params.At(i); isIfaceVar(v) {
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
	// The types of a variable flow into the variables depending on it.
	resolved := map[*types.Var]map[types.Type]bool{}
	for v := range cand {
		if !bad[v] {
			set := map[types.Type]bool{}
			for _, t := range vals[v] {
				set[t] = true
			}
			resolved[v] = set
		}
	}
	for changed := true; changed; {
		changed = false
		for v, set := range resolved {
			for _, d := range deps[v] {
				for t := range resolved[d] {
					if !set[t] {
						set[t] = true
						changed = true
					}
				}
			}
		}
	}
	p.ifaceVars = map[*types.Var][]types.Type{}
	for v, set := range resolved {
		ts := make([]types.Type, 0, len(set))
		for t := range set {
			ts = append(ts, t)
		}
		p.ifaceVars[v] = ts
	}
}

// knownTypes returns the dynamic types the interface value e may have when
// it is a variable, or a call of a declared function whose result is one,
// with known types (findIfaceVars).
func (p *Program) knownTypes(info *types.Info, e ast.Expr) ([]types.Type, bool) {
	var v *types.Var
	switch e := unparen(e).(type) {
	case *ast.Ident:
		v, _ = info.Uses[e].(*types.Var)
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			if sel.Kind() == types.FieldVal {
				v = p.fieldVar(sel.Obj().(*types.Var))
			}
		} else {
			v, _ = info.Uses[e.Sel].(*types.Var)
		}
	case *ast.CallExpr:
		if tv, ok := info.Types[e.Fun]; ok && tv.IsType() {
			return nil, false
		}
		v = resultVar(info, e, 0, 1)
	}
	if v == nil {
		return nil, false
	}
	ts, ok := p.ifaceVars[v]
	return ts, ok
}

// typesCallBlocks reports whether calling method m on a value of one of the
// types ts may block. via is the interface the value is asserted to before
// the call, or nil: a type without the method fails the assertion.
func (p *Program) typesCallBlocks(ts []types.Type, m *types.Func, via types.Type) bool {
	return p.typesCallBlocksIn(ts, m, via, map[*types.Var]bool{})
}

// typesCallBlocksIn is typesCallBlocks, where the embedded interface fields
// seen are being checked already: their types add nothing new.
func (p *Program) typesCallBlocksIn(ts []types.Type, m *types.Func, via types.Type, seen map[*types.Var]bool) bool {
	for _, t := range ts {
		obj, index, _ := types.LookupFieldOrMethod(t, true, m.Pkg(), m.Name())
		fn, ok := obj.(*types.Func)
		switch {
		case !ok && via == nil:
			return true
		case !ok:
		case fn.Signature().Recv() != nil && isIface(fn.Signature().Recv().Type()):
			// Promoted from an embedded interface field, which may have
			// known types itself.
			var f *types.Var
			cur := t
			for _, k := range index[:len(index)-1] {
				if ptr, ok := cur.Underlying().(*types.Pointer); ok {
					cur = ptr.Elem()
				}
				st, ok := cur.Underlying().(*types.Struct)
				if !ok {
					f = nil
					break
				}
				f = st.Field(k)
				cur = f.Type()
			}
			if f != nil {
				fv := p.fieldVar(f)
				if fts, ok := p.ifaceVars[fv]; ok {
					if !seen[fv] {
						seen[fv] = true
						if p.typesCallBlocksIn(fts, m, nil, seen) {
							return true
						}
					}
					continue
				}
			}
			if p.ifaceCallBlocks(fn.Signature().Recv().Type(), fn, map[types.Type]bool{}) {
				return true
			}
		case p.async[fn.Origin()] && p.liveMethod(fn.Origin()):
			// A method reachable code cannot call does not count, as
			// for ifaceCallBlocks.
			return true
		}
	}
	return false
}

// fieldVar returns the variable of field f (findIfaceVars): the field of
// the first identical unnamed struct type, or of the generic type.
func (p *Program) fieldVar(f *types.Var) *types.Var {
	if c, ok := p.unnamedFields[f]; ok {
		return c
	}
	return f.Origin()
}

// dropRecv returns sig without its receiver.
func dropRecv(sig *types.Signature) *types.Signature {
	return types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
}
