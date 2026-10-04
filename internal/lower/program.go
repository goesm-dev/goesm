// Package lower is goesm's semantic lowering: it walks the typed Go AST
// (syntax from go/parser, semantics from go/types, both run by the go
// toolchain via go/packages) and emits one TypeScript module per Go package.
// Go-specific semantics that JavaScript lacks are expressed as calls into
// @goesm/runtime. Go's type checker makes every typing decision; the
// TypeScript types only describe the result (CI checks them with tsc).
package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/goesm-dev/goesm/internal/natives"
)

// Diagnostic is a lowering diagnostic at an original .go position. These are
// goesm limitations ("not yet supported"), never Go compile errors: the
// frontend has already accepted the program.
type Diagnostic struct {
	Pos token.Position
	Msg string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s [goesm lowering]", d.Pos, d.Msg)
}

// Program holds whole-program facts needed while lowering any one package.
type Program struct {
	Fset *token.FileSet
	Pkgs []*packages.Package

	byTypes map[*types.Package]*packages.Package
	std     map[*packages.Package]bool

	// async is the set of functions (declarations by *types.Func, literals by
	// *ast.FuncLit) that may block and are therefore lowered to async
	// functions.
	async map[any]bool
	// syncOnly are blocking-looking functions lowered as synchronous
	// (natives.Sync, and the function literals inside them).
	syncOnly map[any]bool
	// boxed are variables that need a Cell because their address is taken
	// (or, for package variables, because another package assigns them).
	boxed map[*types.Var]bool

	units []*unit
	// funcValues are the functions that can be called through a function
	// value: function literals not called in place, and functions or
	// methods referenced other than as the callee of a call.
	funcValues map[any]bool
	// methodExprs are the function types of method expressions used as
	// values (T.M, (*T).M, I.M), by method name: calling such a value
	// passes the receiver as the first argument.
	methodExprs map[string][]*types.Signature
	// ifaceVals are the interface (and type parameter) methods used as
	// method values (x.M with x an interface): calling such a value calls
	// the method of x's dynamic type.
	ifaceVals []ifaceCall
	// ifaceImpls are the methods of the types stored in interface values,
	// by name (see findIfaceTypes).
	ifaceImpls map[string][]ifaceImpl
	implCache  map[[2]any]bool
	msets      typeutil.MethodSetCache
	// lockCalls are the program's sync.Mutex, RWMutex and Locker Lock and
	// RLock calls; waitLocks those lowered to a waiting variant, and
	// escMutexes the mutexes whose address escapes (lockcheck.go).
	lockCalls  []mutexCall
	waitLocks  map[*ast.CallExpr]*types.Func
	escMutexes map[types.Object]bool
	// lockVals are the Lock and RLock method values and method expression
	// values; waitLockVals those bound to a waiting variant.
	lockVals     []lockVal
	waitLockVals map[*ast.SelectorExpr]*types.Func
	// goexits are the functions that may call runtime.Goexit.
	goexits map[any]bool

	Diags []Diagnostic
	// Warns are standard library functions that were replaced by stubs
	// that panic when called.
	Warns []Diagnostic
}

func (p *Program) errorf(pos token.Pos, format string, args ...any) {
	p.Diags = append(p.Diags, Diagnostic{Pos: p.Fset.Position(pos), Msg: fmt.Sprintf(format, args...)})
}

// NewProgram analyses the loaded packages (dependencies first). std is the
// set of standard library packages.
func NewProgram(fset *token.FileSet, pkgs []*packages.Package, std map[*packages.Package]bool) *Program {
	p := &Program{
		Fset:     fset,
		Pkgs:     pkgs,
		std:      std,
		byTypes:  map[*types.Package]*packages.Package{},
		async:    map[any]bool{},
		syncOnly: map[any]bool{},
		boxed:    map[*types.Var]bool{},

		ifaceImpls:   map[string][]ifaceImpl{},
		implCache:    map[[2]any]bool{},
		waitLocks:    map[*ast.CallExpr]*types.Func{},
		waitLockVals: map[*ast.SelectorExpr]*types.Func{},
	}
	for _, pkg := range pkgs {
		p.byTypes[pkg.Types] = pkg
	}
	p.analyzeAddrs()
	p.analyzeBlocking()
	return p
}

func isAggregate(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

func unparen(e ast.Expr) ast.Expr { return ast.Unparen(e) }

// analyzeAddrs finds variables whose address is taken explicitly (&x) or
// implicitly (x.M() with a pointer receiver), and package variables assigned
// from other packages (ES module bindings are read-only for importers).
func (p *Program) analyzeAddrs() {
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		varOf := func(e ast.Expr) *types.Var {
			switch e := unparen(e).(type) {
			case *ast.Ident:
				v, _ := info.Uses[e].(*types.Var)
				if v == nil {
					v, _ = info.Defs[e].(*types.Var)
				}
				return v
			case *ast.SelectorExpr:
				if _, ok := info.Selections[e]; !ok {
					v, _ := info.Uses[e.Sel].(*types.Var)
					return v
				}
			}
			return nil
		}
		box := func(v *types.Var) {
			if v != nil && !v.IsField() && !isAggregate(v.Type()) {
				p.boxed[v] = true
			}
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						box(varOf(n.X))
					}
				case *ast.SelectorExpr:
					sel, ok := info.Selections[n]
					if !ok || sel.Kind() != types.MethodVal || len(sel.Index()) != 1 {
						return true
					}
					fn := sel.Obj().(*types.Func)
					recv := fn.Signature().Recv()
					if recv == nil {
						return true
					}
					if _, isPtr := recv.Type().(*types.Pointer); isPtr {
						if _, xPtr := info.TypeOf(n.X).Underlying().(*types.Pointer); !xPtr {
							box(varOf(n.X))
						}
					}
				case *ast.AssignStmt:
					for _, l := range n.Lhs {
						if v := varOf(l); v != nil && v.Pkg() != pkg.Types && v.Parent() == v.Pkg().Scope() {
							box(v)
						}
					}
				case *ast.IncDecStmt:
					if v := varOf(n.X); v != nil && v.Pkg() != pkg.Types && v.Parent() == v.Pkg().Scope() {
						box(v)
					}
				}
				return true
			})
		}
	}
}

// Blocking analysis. A Go function is lowered to an async JS function iff it
// may block: it performs a channel operation or select, calls (or defers) a
// function that may block, or makes a dynamic call (function value or
// interface method) that may reach one. Function values are resolved
// conservatively by signature over the whole program, interface methods by
// the types stored in interface values (see findIfaceTypes).
type unit struct {
	key        any // *types.Func or *ast.FuncLit
	sig        *types.Signature
	isMethod   bool
	name       string
	blocking   bool
	callees    []any
	dynSigs    []*types.Signature
	ifaceCalls []ifaceCall
	lockCalls  []*ast.CallExpr // sync Lock and RLock calls (lockcheck.go)
	goexit     bool            // calls runtime.Goexit
}

func (p *Program) analyzeBlocking() {
	var units []*unit
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if n.Body == nil {
						return false
					}
					fn := info.Defs[n.Name].(*types.Func)
					if natives.Sync(fn.FullName()) && p.std[pkg] {
						p.syncOnly[fn] = true
						ast.Inspect(n.Body, func(m ast.Node) bool {
							switch m := m.(type) {
							case *ast.FuncLit:
								p.syncOnly[m] = true
							case *ast.RangeStmt:
								p.syncOnly[m] = true
							}
							return true
						})
					}
					units = append(units, p.scanUnit(pkg, fn, fn.Signature(), n.Recv != nil, fn.Name(), n.Body))
				case *ast.FuncLit:
					sig, _ := info.TypeOf(n).(*types.Signature)
					units = append(units, p.scanUnit(pkg, n, sig, false, "", n.Body))
				case *ast.RangeStmt:
					// A range-over-func body is the yield function passed
					// to the iterator: its own unit, and a function value.
					if sig, ok := info.TypeOf(n.X).Underlying().(*types.Signature); ok && sig.Params().Len() == 1 {
						if yield, ok := sig.Params().At(0).Type().Underlying().(*types.Signature); ok {
							units = append(units, p.scanUnit(pkg, n, yield, false, "", n.Body))
						}
					}
				}
				return true
			})
		}
	}
	p.units = units
	p.funcValues, p.methodExprs = p.findFuncValues()
	for _, u := range units {
		if _, ok := u.key.(*ast.RangeStmt); ok {
			p.funcValues[u.key] = true
		}
	}
	p.findIfaceTypes()
	p.propagateBlocking()
	for p.findWaitLocks() {
		p.propagateBlocking()
	}
}

func (p *Program) propagateBlocking() {
	for changed := true; changed; {
		changed = false
		for _, u := range p.units {
			if p.async[u.key] {
				continue
			}
			if p.unitBlocks(u, p.units) {
				p.async[u.key] = true
				changed = true
			}
		}
	}
}

func (p *Program) findFuncValues() (map[any]bool, map[string][]*types.Signature) {
	vals := map[any]bool{}
	exprs := map[string][]*types.Signature{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			callees := map[ast.Expr]bool{}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					fun := unparen(n.Fun)
					switch g := fun.(type) {
					case *ast.IndexExpr:
						fun = unparen(g.X)
					case *ast.IndexListExpr:
						fun = unparen(g.X)
					}
					callees[fun] = true
					if sel, ok := fun.(*ast.SelectorExpr); ok {
						callees[sel.Sel] = true
					}
				case *ast.FuncLit:
					if !callees[n] {
						vals[n] = true
					}
				case *ast.SelectorExpr:
					if !callees[n] {
						if sel, ok := info.Selections[n]; ok && sel.Kind() == types.MethodExpr {
							sig, _ := info.TypeOf(n).Underlying().(*types.Signature)
							if sig != nil {
								exprs[n.Sel.Name] = append(exprs[n.Sel.Name], sig)
							}
							p.addLockVal(pkg, n, nil)
						} else if fn, ok := info.Uses[n.Sel].(*types.Func); ok {
							vals[fn.Origin()] = true
							if sel != nil && sel.Kind() == types.MethodVal {
								p.addLockVal(pkg, n, n.X)
								if recv := fn.Signature().Recv().Type(); isIface(recv) {
									if tp, ok := types.Unalias(sel.Recv()).(*types.TypeParam); ok {
										recv = tp
									}
									p.ifaceVals = append(p.ifaceVals, ifaceCall{recv, fn})
								}
							}
						}
						callees[n.Sel] = true // the Sel ident is visited next
					}
				case *ast.Ident:
					if !callees[n] {
						if fn, ok := info.Uses[n].(*types.Func); ok {
							vals[fn.Origin()] = true
						}
					}
				}
				return true
			})
		}
	}
	return vals, exprs
}

// SyncOnly reports whether a function (*types.Func) or literal is lowered as
// synchronous although it contains channel operations (see natives.Sync).
func (p *Program) SyncOnly(key any) bool { return p.syncOnly[key] }

func (p *Program) unitBlocks(u *unit, units []*unit) bool {
	if p.syncOnly[u.key] {
		return false
	}
	if u.blocking {
		return true
	}
	for _, c := range u.callees {
		if p.async[c] {
			return true
		}
	}
	for _, s := range u.dynSigs {
		for _, o := range units {
			if !p.async[o.key] || o.sig == nil {
				continue
			}
			if p.funcValues[o.key] && sigMatch(s, o.sig) {
				return true
			}
			// A method reached through a method expression value: the
			// receiver is the first parameter (an interface for I.M).
			if o.isMethod {
				for _, e := range p.methodExprs[o.name] {
					if sigMatch(s, e) && sigMatch(dropFirstParam(e), o.sig) {
						return true
					}
				}
			}
		}
	}
	for _, s := range u.dynSigs {
		for _, lv := range p.lockVals {
			if p.waitLockVals[lv.sel] != nil && sigMatch(s, lv.sig) {
				return true
			}
		}
		for _, c := range p.ifaceVals {
			if sigMatch(s, c.fn.Signature()) && p.ifaceCallBlocks(c.recv, c.fn, map[types.Type]bool{}) {
				return true
			}
		}
	}
	for _, c := range u.lockCalls {
		if p.waitLocks[c] != nil {
			return true
		}
	}
	for _, c := range u.ifaceCalls {
		if p.ifaceCallBlocks(c.recv, c.fn, map[types.Type]bool{}) {
			return true
		}
	}
	return false
}

func (p *Program) scanUnit(pkg *packages.Package, key any, sig *types.Signature, isMethod bool, name string, body *ast.BlockStmt) *unit {
	info := pkg.TypesInfo
	u := &unit{key: key, sig: sig, isMethod: isMethod, name: name}
	var inGo map[*ast.CallExpr]bool
	// The channel operations of a select with a default case never wait.
	var nowait map[ast.Node]bool
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false // separate unit
		case *ast.GoStmt:
			if inGo == nil {
				inGo = map[*ast.CallExpr]bool{}
			}
			inGo[n.Call] = true
		case *ast.SendStmt:
			if !nowait[n] {
				u.blocking = true
			}
		case *ast.UnaryExpr:
			if n.Op == token.ARROW && !nowait[n] {
				u.blocking = true
			}
		case *ast.SelectStmt:
			hasDefault := false
			for _, c := range n.Body.List {
				if c.(*ast.CommClause).Comm == nil {
					hasDefault = true
				}
			}
			if !hasDefault {
				u.blocking = true
				break
			}
			if nowait == nil {
				nowait = map[ast.Node]bool{}
			}
			for _, c := range n.Body.List {
				switch comm := c.(*ast.CommClause).Comm.(type) {
				case *ast.SendStmt:
					nowait[comm] = true
				case *ast.ExprStmt:
					nowait[unparen(comm.X)] = true
				case *ast.AssignStmt:
					nowait[unparen(comm.Rhs[0])] = true
				}
			}
		case *ast.RangeStmt:
			switch info.TypeOf(n.X).Underlying().(type) {
			case *types.Chan:
				u.blocking = true
			case *types.Signature: // range-over-func calls the iterator
				p.classifyFunc(info, n.X, u)
			}
		case *ast.CallExpr:
			// A `go` call's callee runs on its own goroutine; its arguments
			// are still evaluated here and are visited as children.
			p.classifyCall(info, n, u, inGo[n])
			if fn, ok := info.Uses[identOf(unparen(n.Fun))].(*types.Func); ok && isGoexit(fn) && !inGo[n] {
				u.goexit = true
			}
			if mc, ok := mutexMethod(info, n); ok && mc.slow != nil && !isSyncPkg(pkg) {
				p.lockCalls = append(p.lockCalls, mc)
				if !inGo[n] {
					u.lockCalls = append(u.lockCalls, n)
				}
			}
		}
		return true
	})
	return u
}

// isSyncPkg reports whether pkg is the sync package, whose own locking
// (rlocker, Cond.Wait, lockerSlow) is written for goesm's locks and left out
// of lockcheck.go's analysis.
func isSyncPkg(pkg *packages.Package) bool { return pkg.PkgPath == "sync" }

func (p *Program) classifyCall(info *types.Info, call *ast.CallExpr, u *unit, isGo bool) {
	if isGo {
		return
	}
	if p.waitLocks[call] != nil {
		u.blocking = true
		return
	}
	p.classifyFunc(info, call.Fun, u)
}

// classifyFunc records a call of the function expression fun in u.
func (p *Program) classifyFunc(info *types.Info, fun ast.Expr, u *unit) {
	callee := fun
	fun = unparen(fun)
	if tv, ok := info.Types[fun]; ok && tv.IsType() {
		return // conversion
	}
	// Strip explicit instantiation.
	switch f := fun.(type) {
	case *ast.IndexExpr:
		if _, ok := info.Instances[identOf(f.X)]; ok {
			fun = f.X
		}
	case *ast.IndexListExpr:
		fun = f.X
	}
	switch f := unparen(fun).(type) {
	case *ast.FuncLit:
		u.callees = append(u.callees, f)
		return
	case *ast.Ident:
		switch obj := info.Uses[f].(type) {
		case *types.Builtin:
			return
		case *types.Func:
			u.callees = append(u.callees, obj.Origin())
			return
		}
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[f]; ok {
			switch sel.Kind() {
			case types.MethodVal, types.MethodExpr:
				fn := sel.Obj().(*types.Func)
				if recv := fn.Signature().Recv().Type(); isIface(recv) {
					if tp, ok := types.Unalias(sel.Recv()).(*types.TypeParam); ok {
						recv = tp
					}
					u.ifaceCalls = append(u.ifaceCalls, ifaceCall{recv, fn})
				} else {
					u.callees = append(u.callees, fn.Origin())
				}
				return
			}
		} else if obj, ok := info.Uses[f.Sel].(*types.Func); ok {
			u.callees = append(u.callees, obj.Origin())
			return
		}
	}
	if sig, ok := info.TypeOf(callee).Underlying().(*types.Signature); ok {
		u.dynSigs = append(u.dynSigs, sig)
	}
}

func identOf(e ast.Expr) *ast.Ident {
	switch e := unparen(e).(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil
}

// dropFirstParam returns sig without its first parameter (the receiver of a
// method expression's function type).
func dropFirstParam(sig *types.Signature) *types.Signature {
	var params []*types.Var
	for i := 1; i < sig.Params().Len(); i++ {
		params = append(params, sig.Params().At(i))
	}
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), sig.Results(), sig.Variadic())
}

func sigMatch(a, b *types.Signature) bool {
	if a.Params().Len() != b.Params().Len() || a.Results().Len() != b.Results().Len() || a.Variadic() != b.Variadic() {
		return false
	}
	if hasTypeParam(a) || hasTypeParam(b) {
		return true // conservative under type parameters
	}
	return types.Identical(types.NewSignatureType(nil, nil, nil, a.Params(), a.Results(), a.Variadic()),
		types.NewSignatureType(nil, nil, nil, b.Params(), b.Results(), b.Variadic()))
}

// hasTypeParam reports whether t mentions a type parameter.
func hasTypeParam(t types.Type) bool {
	seen := map[types.Type]bool{}
	var walk func(t types.Type) bool
	walk = func(t types.Type) bool {
		if t == nil || seen[t] {
			return false
		}
		seen[t] = true
		switch t := types.Unalias(t).(type) {
		case *types.TypeParam:
			return true
		case *types.Named:
			for i := 0; i < t.TypeArgs().Len(); i++ {
				if walk(t.TypeArgs().At(i)) {
					return true
				}
			}
			return false
		case *types.Pointer:
			return walk(t.Elem())
		case *types.Slice:
			return walk(t.Elem())
		case *types.Array:
			return walk(t.Elem())
		case *types.Chan:
			return walk(t.Elem())
		case *types.Map:
			return walk(t.Key()) || walk(t.Elem())
		case *types.Tuple:
			for i := 0; i < t.Len(); i++ {
				if walk(t.At(i).Type()) {
					return true
				}
			}
		case *types.Signature:
			return walk(t.Params()) || walk(t.Results())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				if walk(t.Field(i).Type()) {
					return true
				}
			}
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				if walk(t.Method(i).Type()) {
					return true
				}
			}
		}
		return false
	}
	return walk(t)
}

// CallBlocks reports whether the call may block (and must be awaited).
func (p *Program) CallBlocks(info *types.Info, call *ast.CallExpr) bool {
	u := &unit{}
	p.classifyCall(info, call, u, false)
	return p.unitBlocks(u, p.units)
}

// RangeBlocks reports whether a range-over-func statement's call of its
// iterator may block (and must be awaited).
func (p *Program) RangeBlocks(info *types.Info, s *ast.RangeStmt) bool {
	u := &unit{}
	p.classifyFunc(info, s.X, u)
	return p.unitBlocks(u, p.units)
}

// LitAsync reports whether a function literal may block.
func (p *Program) LitAsync(lit *ast.FuncLit) bool { return p.async[lit] }

// RangeBodyAsync reports whether the body of a range-over-func loop (its
// yield function) may block.
func (p *Program) RangeBodyAsync(s *ast.RangeStmt) bool { return p.async[s] }

// IsAsync reports whether the function declared by fn may block.
func (p *Program) IsAsync(fn *types.Func) bool { return p.async[fn.Origin()] }

// SortedDiags returns diagnostics in position order.
func (p *Program) SortedDiags() []Diagnostic { return sortDiags(p.Diags) }

// SortedWarns returns warnings in position order.
func (p *Program) SortedWarns() []Diagnostic { return sortDiags(p.Warns) }

func sortDiags(diags []Diagnostic) []Diagnostic {
	d := append([]Diagnostic(nil), diags...)
	sort.SliceStable(d, func(i, j int) bool {
		a, b := d[i].Pos, d[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return d
}
