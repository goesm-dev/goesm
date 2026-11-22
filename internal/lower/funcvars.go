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
//     linked with //go:linkname or called from JavaScript as an export, or
//   - a result of function type of a function declared with a body,
//
// whose address is never taken, which is not an exported field of a struct
// that reflection can set, and every value of which, assigned, stored
// in a composite literal, passed as an argument or returned, is a function
// literal, a declared function, nil, or another such variable, or a call of
// a declared function whose result is one (var f = sync.OnceFunc(g)), whose
// values it then holds too.
//
// This keeps, for example, a component's update := func() {...} called from
// its event handlers synchronous in a program where some other func() value
// blocks, and Pool.Get synchronous unless some pool's New blocks. The
// function values only such variables hold do not count for the calls of
// other function values either (findContainedFuncs).

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
	// A value of an unnamed struct type is assignable to a named struct
	// type of the same underlying type, which then holds its function
	// fields without a store to them: the function fields of the named
	// types some unnamed struct type of the program is identical to are
	// not known.
	var unnamed []*types.Struct
	var named []types.Type
	// shared are the struct types whose function fields other types'
	// fields hold too, unknown there (containedFuncs).
	var shared []types.Type
	seenType := map[types.Type]bool{}
	hasFuncField := func(st *types.Struct) bool {
		for i := 0; i < st.NumFields(); i++ {
			if isFunc(st.Field(i)) {
				return true
			}
		}
		return false
	}
	addType := func(t types.Type) {
		if t == nil || seenType[t] {
			return
		}
		seenType[t] = true
		switch t := t.(type) {
		case *types.Struct:
			if hasFuncField(t) {
				unnamed = append(unnamed, t)
			}
		case *types.Named:
			if st, ok := t.Underlying().(*types.Struct); ok && hasFuncField(st) {
				named = append(named, t)
			}
		}
	}
	for _, pkg := range p.Pkgs {
		for _, tv := range pkg.TypesInfo.Types {
			addType(tv.Type)
		}
		for _, obj := range pkg.TypesInfo.Defs {
			if obj != nil {
				addType(obj.Type())
			}
		}
		for _, inst := range pkg.TypesInfo.Instances {
			addType(inst.Type)
		}
	}
	// The struct type of a type declaration is the named type's
	// underlying type, not a type of values.
	declared := map[types.Type]bool{}
	for _, n := range named {
		declared[n.Underlying()] = true
		if o := n.(*types.Named).Origin(); o != nil {
			declared[o.Underlying()] = true
		}
	}
	for _, n := range named {
		for _, u := range unnamed {
			if !declared[u] && types.Identical(n.Underlying(), u) {
				badFields(n)
				shared = append(shared, n)
				break
			}
		}
	}

	// Reflection sets the exported fields of function type of the structs
	// it reaches through a pointer from an interface value (findIfaceTypes)
	// without a store to the field, to a function value it can get: one Go
	// code stored in an interface value, an element of a slice, array, map
	// or channel it reaches, or the value of a field of identical type it
	// reaches. The first two are not known; a field set from a field holds
	// the other field's values.
	if p.reflectSets() {
		var settable, readable []*types.Var
		loose := p.ifaceFuncs
		fields := func(st *types.Struct, set bool) {
			for i := 0; i < st.NumFields(); i++ {
				if f := st.Field(i); f.Exported() && isFunc(f) {
					readable = append(readable, f.Origin())
					if set {
						settable = append(settable, f.Origin())
					}
				}
			}
		}
		p.ifaceTypes.Iterate(func(t types.Type, _ any) {
			switch u := t.Underlying().(type) {
			case *types.Pointer:
				if st, ok := u.Elem().Underlying().(*types.Struct); ok {
					fields(st, true)
				}
			case *types.Struct:
				fields(u, false)
			case *types.Slice:
				loose = append(loose, u.Elem())
			case *types.Array:
				loose = append(loose, u.Elem())
			case *types.Map:
				loose = append(loose, u.Key(), u.Elem())
			case *types.Chan:
				loose = append(loose, u.Elem())
			}
		})
		for _, f := range settable {
			for _, t := range loose {
				if types.Identical(t.Underlying(), f.Type().Underlying()) {
					bad[f] = true
					break
				}
			}
			if bad[f] {
				continue
			}
			for _, r := range readable {
				if r != f && types.Identical(r.Type().Underlying(), f.Type().Underlying()) {
					deps[f] = append(deps[f], r)
				}
			}
		}
	}

	// JavaScript gives the function fields of the structs an entry's ABI
	// carries functions of its own, or functions Go returned to it.
	for _, f := range p.entryABIFuncFields() {
		bad[f] = true
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

	// The results of function type of the functions declared with a body
	// hold the values their return statements return (named results are
	// assigned too); a call of such a function gives those values.
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
				if !ok || fd.Body == nil {
					continue
				}
				fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				results := fn.Signature().Results()
				has := false
				for i := 0; i < results.Len(); i++ {
					if v := results.At(i); isFunc(v) {
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
							if v := results.At(i); isFunc(v) && len(n.Results) > 0 {
								rets = append(rets, ret{pkg.TypesInfo, v, n.Results, i, results.Len()})
							}
						}
					}
					return true
				})
			}
		}
	}

	// stored are the expressions stored in function variables, and lhs
	// the expressions assigned to (containedFuncs).
	stored := map[ast.Expr]*types.Var{}
	lhs := map[ast.Expr]bool{}
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
			stored[unparen(rhs)] = v
			var id *ast.Ident
			switch e := unparen(rhs).(type) {
			case *ast.FuncLit:
				vals[v] = append(vals[v], e)
				return
			case *ast.Ident:
				id = e
			case *ast.SelectorExpr:
				if sel, ok := info.Selections[e]; ok {
					switch fn, _ := sel.Obj().(*types.Func); {
					case sel.Kind() == types.FieldVal:
						deps[v] = append(deps[v], sel.Obj().(*types.Var).Origin())
					case fn != nil && !isIface(fn.Signature().Recv().Type()) && !(fn.Pkg() != nil && fn.Pkg().Path() == "sync" && isLock(fn.Name())):
						// A method value or expression of a concrete
						// type calls that method, as classifyFunc
						// takes a call of it to. The value of a Lock
						// of a mutex may be its waiting variant
						// (waitLockVals), which lockcheck.go picks
						// later.
						vals[v] = append(vals[v], fn.Origin())
					default:
						bad[v] = true // a method of an interface, or a Lock
					}
					return
				}
				id = e.Sel
			case *ast.CallExpr:
				// The function value a declared function returns.
				if r := resultVar(info, e, 0, 1); r != nil {
					deps[v] = append(deps[v], r)
				} else {
					bad[v] = true
				}
				return
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
							storeResult(v, n.Values, i, len(n.Names))
						}
					}
				case *ast.AssignStmt:
					for i, l := range n.Lhs {
						lhs[unparen(l)] = true
						if n.Tok == token.DEFINE {
							if id, ok := l.(*ast.Ident); ok {
								if v, ok := info.Defs[id].(*types.Var); ok && isFunc(v) {
									cand[v] = true
								}
							}
						}
						if len(n.Lhs) == len(n.Rhs) {
							assign(l, n.Rhs[i])
						} else if v := varOf(l); v != nil && isFunc(v) {
							storeResult(v, n.Rhs, i, len(n.Lhs))
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
						if len(n.Args) == 1 {
							if t := info.TypeOf(n.Args[0]); t != nil {
								shared = append(shared, t)
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
	p.containedFuncs = p.findContainedFuncs(resolved, stored, lhs, called, shared)
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

// reflectSets reports whether the program uses a function of package
// reflect that stores a value into what a reflect.Value refers to.
func (p *Program) reflectSets() bool {
	for _, pkg := range p.Pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			fn, ok := obj.(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "reflect" {
				continue
			}
			switch fn.FullName() {
			case "(reflect.Value).Set", "(reflect.Value).SetIterKey", "(reflect.Value).SetIterValue", "reflect.Copy":
				return true
			}
		}
	}
	return false
}

// entryABIFuncFields returns the function fields of the struct types that
// the JavaScript ABI of the entry packages carries, in the arguments and
// results of their exported functions and of the methods of their handles,
// and of the methods JavaScript calls on handles that cross inside interface
// values, and in what those contain. Before SetEntries there are none.
func (p *Program) entryABIFuncFields() []*types.Var {
	fields, _ := p.entryABI()
	return fields
}

// entryABICalls returns the functions and methods JavaScript calls through
// the ABI of the entry packages (see entryABIFuncFields), by their origin.
// Their arguments are JavaScript's, and JavaScript gets their results.
// Before SetEntries there are none.
func (p *Program) entryABICalls() map[*types.Func]bool {
	_, calls := p.entryABI()
	return calls
}

// entryABI returns entryABIFuncFields and entryABICalls.
func (p *Program) entryABI() ([]*types.Var, map[*types.Func]bool) {
	var out []*types.Var
	calls := map[*types.Func]bool{}
	seen := map[types.Type]bool{}
	var walk func(t types.Type)
	walkSig := func(sig *types.Signature) {
		for i := 0; i < sig.Params().Len(); i++ {
			walk(sig.Params().At(i).Type())
		}
		for i := 0; i < sig.Results().Len(); i++ {
			walk(sig.Results().At(i).Type())
		}
	}
	walkMethods := func(n *types.Named) {
		for _, fn := range handleMethods(n) {
			calls[fn.Origin()] = true
			walkSig(fn.Signature())
		}
	}
	walk = func(t types.Type) {
		t = types.Unalias(t)
		if seen[t] || isJSValue(t) {
			return
		}
		seen[t] = true
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			if n, ok := types.Unalias(u.Elem()).(*types.Named); ok && isHandleType(t) {
				walkMethods(n)
			}
			walk(u.Elem())
		case *types.Slice:
			walk(u.Elem())
		case *types.Array:
			walk(u.Elem())
		case *types.Map:
			walk(u.Key())
			walk(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if f := u.Field(i); f.Exported() || promotes(f) {
					if isSignature(f.Type()) {
						out = append(out, f.Origin())
					}
					walk(f.Type())
				}
			}
		case *types.Signature:
			walkSig(u)
		}
	}
	for _, pkg := range p.Pkgs {
		if !p.Entries[pkg.PkgPath] {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			switch obj := scope.Lookup(name).(type) {
			case *types.Func:
				if obj.Exported() && obj.Signature().TypeParams().Len() == 0 {
					calls[obj] = true
					walkSig(obj.Signature())
				}
			case *types.TypeName:
				if n, ok := obj.Type().(*types.Named); ok && obj.Exported() && n.TypeParams().Len() == 0 {
					walkMethods(n)
				}
			}
		}
	}
	// The handles that cross inside interface values have methods
	// JavaScript calls too (anyHandleMethods), which no signature above
	// names.
	if len(p.Entries) > 0 {
		for fn := range p.anyHandleMethods() {
			calls[fn.Origin()] = true
			walkSig(fn.Signature())
		}
	}
	return out, calls
}

// resultVar returns result i of the function call calls, when it is a
// declared function or a method of a concrete type with n results, or nil.
// The result is the variable of the generic function for an instance.
func resultVar(info *types.Info, call *ast.CallExpr, i, n int) *types.Var {
	fun := unparen(call.Fun)
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
			if sel.Kind() != types.MethodVal {
				return nil
			}
			fn, _ = sel.Obj().(*types.Func)
		} else {
			fn, _ = info.Uses[f.Sel].(*types.Func)
		}
	}
	if fn == nil {
		return nil
	}
	sig := fn.Origin().Signature()
	if r := sig.Recv(); r != nil && isIface(r.Type()) || sig.Results().Len() != n {
		return nil
	}
	return sig.Results().At(i)
}

// findContainedFuncs returns the function values that no call of an unknown
// function value can call (dynSigBlocks): each use of them as a value
// stores them in a function variable with known values (resolved), and
// each use of such a variable that holds them is a call of it, a store of
// it in another such variable, an assignment to it or a comparison with
// nil. A blocking func(string) string kept in a package variable then does
// not make every call of some other func(string) string value block. The
// exported fields leak their values: reflection reads them.
func (p *Program) findContainedFuncs(resolved map[*types.Var]map[any]bool, stored map[ast.Expr]*types.Var, lhs map[ast.Expr]bool, called map[*ast.Ident]bool, shared []types.Type) map[any]bool {
	escapes := map[any]bool{}
	leaks := map[*types.Var]bool{}
	known := func(e ast.Expr) bool {
		v := stored[e]
		return v != nil && resolved[v] != nil
	}
	// JavaScript reads the exported package variables of an entry too.
	for v := range resolved {
		if v.Exported() && (v.IsField() || v.Parent() == v.Pkg().Scope() && p.Entries[v.Pkg().Path()]) {
			leaks[v] = true
		}
	}
	// The fields a conversion or an identical unnamed struct type reads
	// as fields of another type.
	for _, t := range shared {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		if st, ok := t.Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				leaks[st.Field(i).Origin()] = true
			}
		}
	}
	// JavaScript gets the results of the functions and methods an entry
	// exports and of the methods of the handles it gets (entryABICalls),
	// and the arguments of the functions it implements (a //goesm:import or
	// a native, declared without a body), and may pass them back.
	jsCalls := p.entryABICalls()
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
				if fd.Body == nil {
					params := fn.Signature().Params()
					for i := 0; i < params.Len(); i++ {
						leaks[params.At(i)] = true
					}
				}
				if !(p.Entries[pkg.PkgPath] && fn.Exported() || jsCalls[fn]) {
					continue
				}
				res := fn.Signature().Results()
				for i := 0; i < res.Len(); i++ {
					leaks[res.At(i)] = true
				}
			}
		}
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			calleeLit := map[ast.Node]bool{}
			nilCmp := map[ast.Expr]bool{}
			parent := map[*ast.Ident]ast.Expr{}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					if lit, ok := unparen(n.Fun).(*ast.FuncLit); ok {
						calleeLit[lit] = true
					}
					// A call of a function whose function results are
					// known gives those results.
					if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
						break
					}
					if t, ok := info.TypeOf(n).(*types.Tuple); ok {
						for i := 0; i < t.Len(); i++ {
							if r := resultVar(info, n, i, t.Len()); r != nil && resolved[r] != nil {
								leaks[r] = true
							}
						}
					} else if r := resultVar(info, n, 0, 1); r != nil && resolved[r] != nil && !known(n) {
						leaks[r] = true
					}
				case *ast.BinaryExpr:
					if n.Op == token.EQL || n.Op == token.NEQ {
						for _, pair := range [2][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
							if id, ok := unparen(pair[1]).(*ast.Ident); ok {
								if _, isNil := info.Uses[id].(*types.Nil); isNil {
									nilCmp[unparen(pair[0])] = true
								}
							}
						}
					}
				case *ast.FuncLit:
					if !calleeLit[n] && !known(n) {
						escapes[n] = true
					}
				case *ast.SelectorExpr:
					parent[n.Sel] = n
				case *ast.Ident:
					if called[n] {
						break
					}
					var e ast.Expr = n
					if s, ok := parent[n]; ok {
						e = s
					}
					switch obj := info.Uses[n].(type) {
					case *types.Func:
						if !known(e) {
							escapes[obj.Origin()] = true
						}
					case *types.Var:
						v := obj.Origin()
						if resolved[v] != nil && !lhs[e] && !nilCmp[e] && !known(e) {
							leaks[v] = true
						}
					}
				}
				return true
			})
		}
	}
	for v := range leaks {
		for fn := range resolved[v] {
			escapes[fn] = true
		}
	}
	// Methods stay function values: reflection makes method values of
	// them too.
	contained := map[any]bool{}
	for _, set := range resolved {
		for fn := range set {
			if f, ok := fn.(*types.Func); ok && f.Signature().Recv() != nil {
				continue
			}
			if !escapes[fn] {
				contained[fn] = true
			}
		}
	}
	return contained
}
