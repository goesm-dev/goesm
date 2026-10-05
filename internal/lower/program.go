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
	"strings"
	"sync"

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
	// tracked, paramCalls and paramUses are the tracked parameters of
	// declared functions, the calls through them and their uses; asyncA
	// the functions that block even when their parameter uses do not, and
	// cloned those that get a synchronous clone (paramsync.go).
	tracked    map[*types.Func]map[*types.Var]trackedVar
	paramCalls map[*ast.CallExpr]paramCall
	paramUses  map[*types.Func][]map[string]paramUse
	asyncA     map[*types.Func]bool
	cloned     map[*types.Func]bool
	// nonEsc are the tracked function parameters that are only called,
	// compared with nil or passed on as such parameters; argOnly the
	// function literals, functions and method values only passed as them,
	// which are not function values; sitesOf the static calls of each
	// function, and unsited the functions also called elsewhere (in go
	// statements and package variable initializers); localLits the local
	// variables holding a literal only passed as them.
	nonEsc    map[*types.Func]map[int]bool
	argOnly   map[ast.Expr]bool
	localLits map[*types.Var]*ast.FuncLit
	sitesOf   map[*types.Func][]siteRef
	unsited   map[*types.Func]bool
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
	// instArgs are the type arguments of the instantiations of each generic
	// function; encls the declared function each call and range statement
	// is in (see unit.encl).
	instArgs map[*types.Func][]*types.TypeList
	encls    map[ast.Node]*types.Func
	// pullCalls are the calls of the next and stop functions of iter.Pull
	// and Pull2 calls whose results are only called (see findPullCalls);
	// pullKept the coroutine functions some of whose results go elsewhere.
	pullCalls map[*ast.CallExpr]bool
	pullKept  map[string]bool
	// usesPull is set when the program calls iter.Pull or Pull2: function
	// literals that are sequences then have generators (seqgen.go).
	usesPull bool

	// //go:linkname pulls (bodyless functions) and the functions providing
	// their symbols (see linkname.go).
	linkPulls    map[*types.Func]string
	linkProvides map[*types.Func]string
	linkTargets  map[*types.Func]*types.Func

	// ifaceMethods are the methods of the program's interface types, by
	// name, and allMethods is set when reflection enumerates methods; they
	// decide which methods method tables list (see methods.go).
	ifaceMethods map[string][]ifaceMethod
	allMethods   bool
	// calledMethods are the interface methods that the code reachable
	// from the program's roots calls, by name: a table lists the other
	// methods of ifaceMethods without their functions (see reach.go).
	calledMethods map[string][]ifaceMethod
	reachOnce     sync.Once
	// unicodeClasses is set when the program may parse \p or \P in a
	// regular expression (UnicodeClasses).
	unicodeClasses bool

	// pureEmitters answer returnsPure for the functions of imported
	// packages (PureEmitter).
	pureEmitters map[*types.Package]*pkgEmitter

	// Entry is the path of the package a build starts from, whose exported
	// functions and methods JavaScript calls (Options.Entry); see reach.go.
	Entry string

	// TracksGoroutines is set when the program uses goroutine-local storage
	// (runtime.GetTraceContextFromGLS and friends): async functions then
	// restore the running goroutine after every await.
	TracksGoroutines bool

	// Deps are the packages of third-party modules (see loader.Program).
	// Like the standard library, their functions that goesm cannot lower
	// become stubs that panic when called.
	Deps map[*packages.Package]bool

	Diags []Diagnostic
	// Warns are standard library and dependency functions that were
	// replaced by stubs that panic when called.
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

		tracked:    map[*types.Func]map[*types.Var]trackedVar{},
		paramCalls: map[*ast.CallExpr]paramCall{},
		paramUses:  map[*types.Func][]map[string]paramUse{},
		asyncA:     map[*types.Func]bool{},
		boxed:      map[*types.Var]bool{},

		ifaceImpls:   map[string][]ifaceImpl{},
		implCache:    map[[2]any]bool{},
		waitLocks:    map[*ast.CallExpr]*types.Func{},
		waitLockVals: map[*ast.SelectorExpr]*types.Func{},
	}
	for _, pkg := range pkgs {
		p.byTypes[pkg.Types] = pkg
	}
	p.analyzeAddrs()
	p.analyzeLinknames()
	p.TracksGoroutines = usesGLS(pkgs)
	p.findDynMethods()
	p.analyzeBlocking()
	return p
}

// usesGLS reports whether a package refers to goroutine-local storage.
func usesGLS(pkgs []*packages.Package) bool {
	for _, pkg := range pkgs {
		if pkg.PkgPath == "runtime" {
			continue
		}
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == "runtime" && strings.HasSuffix(fn.Name(), "GLS") {
				return true
			}
		}
	}
	return false
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
	// sites are the static calls of declared functions, and paramDyn and
	// paramIface the calls through tracked parameters (paramsync.go).
	sites      []callSite
	paramDyn   []paramDyn
	paramIface []ifaceCall
	lockCalls  []*ast.CallExpr // sync Lock and RLock calls (lockcheck.go)
	goexit     bool            // calls runtime.Goexit
	// encl is the declared function whose body the unit is or is in.
	encl *types.Func
}

func (p *Program) analyzeBlocking() {
	p.findPullCalls()
	p.usesPull = p.usesPullFuncs()
	p.encls = map[ast.Node]*types.Func{}
	var units []*unit
	add := func(u *unit, encl *types.Func) {
		u.encl = encl
		units = append(units, u)
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			var encl *types.Func
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if n.Body == nil {
						// A JS function returning a Promise blocks
						// (jsimport.go).
						if d := funcJSImport(n); d != nil && d.await && !p.std[pkg] {
							fn := info.Defs[n.Name].(*types.Func)
							units = append(units, &unit{key: fn, sig: fn.Signature(), name: fn.Name(), blocking: true})
						}
						return false
					}
					fn := info.Defs[n.Name].(*types.Func)
					encl = fn
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
					p.trackParams(info, fn, n)
					add(p.scanUnit(pkg, fn, fn.Signature(), n.Recv != nil, fn.Name(), n.Body), encl)
				case *ast.FuncLit:
					sig, _ := info.TypeOf(n).(*types.Signature)
					add(p.scanUnit(pkg, n, sig, false, "", n.Body), encl)
				case *ast.CallExpr:
					p.encls[n] = encl
				case *ast.RangeStmt:
					p.encls[n] = encl
					// A range-over-func body is the yield function passed
					// to the iterator: its own unit, and a function value.
					if sig, ok := info.TypeOf(n.X).Underlying().(*types.Signature); ok && sig.Params().Len() == 1 {
						if yield, ok := sig.Params().At(0).Type().Underlying().(*types.Signature); ok {
							add(p.scanUnit(pkg, n, yield, false, "", n.Body), encl)
						}
					}
				}
				return true
			})
		}
	}
	p.units = units
	p.instArgs = map[*types.Func][]*types.TypeList{}
	for _, pkg := range p.Pkgs {
		for id, inst := range pkg.TypesInfo.Instances {
			if fn, ok := pkg.TypesInfo.Uses[id].(*types.Func); ok {
				p.instArgs[fn.Origin()] = append(p.instArgs[fn.Origin()], inst.TypeArgs)
			}
		}
	}
	p.findNonEscaping()
	p.funcValues, p.methodExprs = p.findFuncValues()
	for _, u := range units {
		if _, ok := u.key.(*ast.RangeStmt); ok {
			p.funcValues[u.key] = true
		}
	}
	p.findIfaceTypes()
	p.findParamUses()
	p.propagateBlocking()
	for p.findWaitLocks() {
		p.propagateBlocking()
	}
	p.findClones()
}

func (p *Program) propagateBlocking() {
	for changed := true; changed; {
		changed = false
		for _, u := range p.units {
			if !p.async[u.key] && p.unitBlocks(u, p.units) {
				p.async[u.key] = true
				changed = true
			}
			if fn, ok := u.key.(*types.Func); ok && p.async[fn] && !p.asyncA[fn] && p.paramUses[fn] != nil && p.unitBlocksIn(u, p.units, fn) {
				p.asyncA[fn] = true
				changed = true
			}
		}
		for pull, target := range p.linkTargets {
			if !p.async[pull] && p.async[target] {
				p.async[pull] = true
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
					if !callees[n] && !p.argOnly[n] {
						vals[n] = true
					}
				case *ast.SelectorExpr:
					if !callees[n] && p.argOnly[n] {
						// Only the receiver's locking matters.
						if sel, ok := info.Selections[n]; ok && sel.Kind() == types.MethodVal {
							p.addLockVal(pkg, n, n.X)
						}
						callees[n.Sel] = true
					} else if !callees[n] {
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
									p.ifaceVals = append(p.ifaceVals, ifaceCall{ifaceRecv(sel, recv), fn})
								}
							}
						}
						callees[n.Sel] = true // the Sel ident is visited next
					}
				case *ast.Ident:
					if !callees[n] && !p.argOnly[n] {
						if fn, ok := info.Uses[n].(*types.Func); ok {
							vals[fn.Origin()] = true
						}
					}
				}
				return true
			})
		}
	}
	p.dropUnusedCoroutines(vals)
	return vals, exprs
}

// coroutineFuncs are standard library functions whose function literals
// block (iter.Pull's next, stop and yield switch coroutines). Function
// values are matched to dynamic calls by signature, so they would make
// every call of a func() or func(T) bool value async; they count as
// function values only in programs that use these functions.
var coroutineFuncs = map[string]bool{"iter.Pull": true, "iter.Pull2": true}

// coroutineHelpers are the functions that make the next and stop functions
// Pull and Pull2 return for a generator (internal/natives/patch/iter): their
// function literals are function values exactly when Pull's are.
var coroutineHelpers = map[string]string{"iter.pullGen": "iter.Pull", "iter.pull2Gen": "iter.Pull2"}

// findPullCalls finds the calls of iter.Pull and Pull2 whose results are
// assigned to local variables that are only called: those calls are then
// the only ones of the next and stop functions, which block, and the
// function values need not match every call of a func() or func() (V, bool)
// value. The functions of other calls stay function values (pullKept).
func (p *Program) findPullCalls() {
	p.pullCalls = map[*ast.CallExpr]bool{}
	p.pullKept = map[string]bool{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		uses := map[types.Object][]*ast.Ident{}
		for id, obj := range info.Uses {
			uses[obj] = append(uses[obj], id)
		}
		callees := map[*ast.Ident]bool{} // of the calls of Pull and Pull2
		for _, f := range pkg.Syntax {
			funs := map[*ast.Ident]*ast.CallExpr{} // idents called directly
			var pulls []*ast.CallExpr
			lhs := map[*ast.CallExpr][]*ast.Ident{}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					if id, ok := unparen(n.Fun).(*ast.Ident); ok {
						funs[id] = n
					}
					if fn := calledFunc(info, n); fn != nil && fn.Pkg() != pkg.Types && coroutineFuncs[fn.Origin().FullName()] {
						pulls = append(pulls, n)
						callees[identOf(funcIdent(unparen(n.Fun)))] = true
					}
				case *ast.AssignStmt:
					if n.Tok == token.DEFINE && len(n.Lhs) == 2 && len(n.Rhs) == 1 {
						if c, ok := unparen(n.Rhs[0]).(*ast.CallExpr); ok {
							lhs[c] = identList(n.Lhs)
						}
					}
				case *ast.ValueSpec:
					if len(n.Names) == 2 && len(n.Values) == 1 {
						if c, ok := unparen(n.Values[0]).(*ast.CallExpr); ok {
							lhs[c] = n.Names
						}
					}
				}
				return true
			})
			for _, c := range pulls {
				name := calledFunc(info, c).Origin().FullName()
				ids := lhs[c]
				var calls []*ast.CallExpr
				ok := ids != nil
				for _, id := range ids {
					if !ok || id == nil {
						ok = false
						break
					}
					if id.Name == "_" {
						continue
					}
					v, isVar := info.Defs[id].(*types.Var)
					if !isVar || v.Parent() == nil || v.Parent() == pkg.Types.Scope() {
						ok = false
						break
					}
					for _, u := range uses[v] {
						call, called := funs[u]
						if !called {
							ok = false
							break
						}
						calls = append(calls, call)
					}
				}
				if !ok {
					p.pullKept[name] = true
					continue
				}
				for _, call := range calls {
					p.pullCalls[call] = true
				}
			}
		}
		// Pull as a function value: its results go anywhere.
		for id, obj := range info.Uses {
			if fn, ok := obj.(*types.Func); ok && fn.Pkg() != pkg.Types && coroutineFuncs[fn.Origin().FullName()] && !callees[id] {
				p.pullKept[fn.Origin().FullName()] = true
			}
		}
	}
}

// identList returns the identifiers of es, nil for one that is not.
func identList(es []ast.Expr) []*ast.Ident {
	ids := make([]*ast.Ident, len(es))
	for i, e := range es {
		ids[i], _ = e.(*ast.Ident)
	}
	return ids
}

// calledFunc returns the declared function call calls, if it calls one.
func calledFunc(info *types.Info, call *ast.CallExpr) *types.Func {
	fn, _ := info.Uses[identOf(funcIdent(unparen(call.Fun)))].(*types.Func)
	return fn
}

func (p *Program) dropUnusedCoroutines(vals map[any]bool) {
	used := map[string]bool{}
	decls := map[string]*ast.FuncDecl{}
	declPkgs := map[string]*packages.Package{}
	for _, pkg := range p.Pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && fn.Pkg() != pkg.Types && coroutineFuncs[fn.Origin().FullName()] {
				used[fn.Origin().FullName()] = true
			}
		}
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil && fd.Name.Name == "_" {
					// A declaration a natives patch replaced (loader.patch):
					// nothing calls it, so its literals are no values.
					ast.Inspect(fd.Body, func(n ast.Node) bool {
						if lit, ok := n.(*ast.FuncLit); ok {
							delete(vals, lit)
						}
						return true
					})
					continue
				}
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
					if fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func); ok && (coroutineFuncs[fn.FullName()] || coroutineHelpers[fn.FullName()] != "") {
						decls[fn.FullName()] = fd
						declPkgs[fn.FullName()] = pkg
					}
				}
			}
		}
	}
	for name, fd := range decls {
		pull := name
		if h := coroutineHelpers[name]; h != "" {
			pull = h
		}
		if used[pull] {
			if !p.pullKept[pull] {
				// Only called where findPullCalls found: drop the
				// function literals assigned to the results, next and
				// stop. yield is passed to the sequence.
				for _, lit := range resultLits(declPkgs[name], fd) {
					delete(vals, lit)
				}
			}
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok {
				delete(vals, lit)
			}
			return true
		})
	}
}

// resultLits returns the function literals assigned to the named results
// of fd.
func resultLits(pkg *packages.Package, fd *ast.FuncDecl) []*ast.FuncLit {
	info := pkg.TypesInfo
	sig := info.Defs[fd.Name].(*types.Func).Signature()
	results := map[types.Object]bool{}
	for i := 0; i < sig.Results().Len(); i++ {
		results[sig.Results().At(i)] = true
	}
	var lits []*ast.FuncLit
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
			for i, l := range as.Lhs {
				id, ok := l.(*ast.Ident)
				lit, isLit := unparen(as.Rhs[i]).(*ast.FuncLit)
				if ok && isLit && results[info.Uses[id]] {
					lits = append(lits, lit)
				}
			}
		}
		return true
	})
	return lits
}

// SyncOnly reports whether a function (*types.Func) or literal is lowered as
// synchronous although it contains channel operations (see natives.Sync).
func (p *Program) SyncOnly(key any) bool { return p.syncOnly[key] }

func (p *Program) unitBlocks(u *unit, units []*unit) bool {
	return p.unitBlocksIn(u, units, nil)
}

// unitBlocksIn is unitBlocks for the synchronous clone of assume
// (paramsync.go): the calls through assume's tracked parameters, and
// passing them on, do not block. A nil assume is unitBlocks.
func (p *Program) unitBlocksIn(u *unit, units []*unit, assume *types.Func) bool {
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
	for _, s := range u.sites {
		if p.siteBlocks(s, assume) {
			return true
		}
	}
	dynSigs, ifaceCalls := u.dynSigs, u.ifaceCalls
	if assume == nil {
		for _, d := range u.paramDyn {
			if p.nonEsc[d.fn][d.idx] {
				if p.paramBlocks(d.fn, d.idx, d.sig, map[paramKey]bool{}) {
					return true
				}
			} else {
				dynSigs = append(dynSigs[:len(dynSigs):len(dynSigs)], d.sig)
			}
		}
		if len(u.paramIface) > 0 {
			ifaceCalls = append(append([]ifaceCall{}, ifaceCalls...), u.paramIface...)
		}
	}
	for _, s := range dynSigs {
		if p.dynSigBlocks(s, u, units) {
			return true
		}
	}
	for _, c := range u.lockCalls {
		if p.waitLocks[c] != nil {
			return true
		}
	}
	for _, c := range ifaceCalls {
		if p.ifaceCallBlocks(c.recv, c.fn, map[types.Type]bool{}) {
			return true
		}
	}
	return false
}

// dynSigBlocks reports whether a call of a function value of signature s,
// in unit u, may block: whether some function value the program has may.
func (p *Program) dynSigBlocks(s *types.Signature, u *unit, units []*unit) bool {
	for _, o := range units {
		if !p.async[o.key] || o.sig == nil {
			continue
		}
		if p.funcValues[o.key] && p.valueSigMatch(s, u, o) {
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
	if p.waitLocks[call] != nil || p.pullCalls[call] {
		u.blocking = true
		return
	}
	if pc := p.paramCalls[call]; pc.fn != nil {
		c := &unit{}
		p.classifyFunc(info, call.Fun, c)
		for _, sig := range c.dynSigs {
			u.paramDyn = append(u.paramDyn, paramDyn{pc.fn, pc.idx, sig})
		}
		u.paramIface = append(u.paramIface, c.ifaceCalls...)
		return
	}
	if fn := staticCallee(info, call); fn != nil {
		u.sites = append(u.sites, callSite{call, fn, info})
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
					u.ifaceCalls = append(u.ifaceCalls, ifaceCall{ifaceRecv(sel, recv), fn})
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

// ifaceRecv returns the type a selected interface method, declared by the
// interface recv, is called through: the type parameter, or the interface
// that has it (hash.Hash for its Write, not io.Writer, which declares it).
// For a struct embedding the interface it is recv.
func ifaceRecv(sel *types.Selection, recv types.Type) types.Type {
	if tp, ok := types.Unalias(sel.Recv()).(*types.TypeParam); ok {
		return tp
	}
	if isIface(sel.Recv()) {
		return sel.Recv()
	}
	return recv
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
	return sigMatchIn(a, b, nil, nil)
}

// sigMatchIn is sigMatch with a's and b's type parameters bound by ma and
// mb, where they have them.
func sigMatchIn(a, b *types.Signature, ma, mb map[*types.TypeParam]types.Type) bool {
	if a.Params().Len() != b.Params().Len() || a.Results().Len() != b.Results().Len() || a.Variadic() != b.Variadic() {
		return false
	}
	return mayUnify(a.Params(), b.Params(), ma, mb) && mayUnify(a.Results(), b.Results(), ma, mb)
}

// valueSigMatch reports whether a call of a function value of signature s,
// in unit u, may call the function of unit o. The signatures of functions
// that are or are in generic functions are matched for each of the
// instantiations of those functions in the program.
func (p *Program) valueSigMatch(s *types.Signature, u, o *unit) bool {
	if !sigMatch(s, o.sig) {
		return false
	}
	mas, mbs := p.bindings(u.encl, s), p.bindings(o.encl, o.sig)
	if mas == nil && mbs == nil {
		return true
	}
	if mas == nil {
		mas = []map[*types.TypeParam]types.Type{nil}
	}
	if mbs == nil {
		mbs = []map[*types.TypeParam]types.Type{nil}
	}
	for _, ma := range mas {
		for _, mb := range mbs {
			if sigMatchIn(s, o.sig, ma, mb) {
				return true
			}
		}
	}
	return false
}

// bindings returns the bindings of the type parameters of the generic
// function f for each of its instantiations in the program (none when the
// program never calls it), or nil when sig, a signature in f, has none of
// them.
func (p *Program) bindings(f *types.Func, sig *types.Signature) []map[*types.TypeParam]types.Type {
	if f == nil || f.Signature().TypeParams().Len() == 0 || !hasTypeParam(sig) {
		return nil
	}
	tps := f.Signature().TypeParams()
	ms := []map[*types.TypeParam]types.Type{}
	for _, targs := range p.instArgs[f] {
		m := map[*types.TypeParam]types.Type{}
		for i := 0; i < tps.Len() && i < targs.Len(); i++ {
			m[tps.At(i)] = targs.At(i)
		}
		ms = append(ms, m)
	}
	return ms
}

// mayUnify reports whether some instantiation of the type parameters in a
// and b may make them identical: a type parameter matches any type, but
// one that ma or mb binds matches what its binding may be, and the parts
// around type parameters must match. It is conservative for the local types
// of generic functions, structs and interfaces with type parameters, whose
// instances it does not compare.
func mayUnify(a, b types.Type, ma, mb map[*types.TypeParam]types.Type) bool {
	a, b = types.Unalias(a), types.Unalias(b)
	// A binding's type parameters are another function's.
	if tp, ok := a.(*types.TypeParam); ok {
		if t, ok := ma[tp]; ok {
			return mayUnify(t, b, nil, mb)
		}
		return true
	}
	if tp, ok := b.(*types.TypeParam); ok {
		if t, ok := mb[tp]; ok {
			return mayUnify(a, t, ma, nil)
		}
		return true
	}
	if at, ok := a.(*types.Tuple); ok {
		bt := b.(*types.Tuple)
		for i := 0; i < at.Len(); i++ {
			if !mayUnify(at.At(i).Type(), bt.At(i).Type(), ma, mb) {
				return false
			}
		}
		return true
	}
	if !hasTypeParam(a) && !hasTypeParam(b) {
		return types.Identical(a, b)
	}
	switch a := a.(type) {
	case *types.Named:
		b, ok := b.(*types.Named)
		if !ok || a.Origin() != b.Origin() || a.TypeArgs().Len() != b.TypeArgs().Len() {
			return false
		}
		for i := 0; i < a.TypeArgs().Len(); i++ {
			if !mayUnify(a.TypeArgs().At(i), b.TypeArgs().At(i), ma, mb) {
				return false
			}
		}
		return true
	case *types.Pointer:
		b, ok := b.(*types.Pointer)
		return ok && mayUnify(a.Elem(), b.Elem(), ma, mb)
	case *types.Slice:
		b, ok := b.(*types.Slice)
		return ok && mayUnify(a.Elem(), b.Elem(), ma, mb)
	case *types.Array:
		b, ok := b.(*types.Array)
		return ok && a.Len() == b.Len() && mayUnify(a.Elem(), b.Elem(), ma, mb)
	case *types.Chan:
		b, ok := b.(*types.Chan)
		return ok && a.Dir() == b.Dir() && mayUnify(a.Elem(), b.Elem(), ma, mb)
	case *types.Map:
		b, ok := b.(*types.Map)
		return ok && mayUnify(a.Key(), b.Key(), ma, mb) && mayUnify(a.Elem(), b.Elem(), ma, mb)
	case *types.Signature:
		b, ok := b.(*types.Signature)
		return ok && sigMatchIn(a, b, ma, mb)
	case *types.Basic:
		return false // b has type parameters, which a basic type has not
	}
	return true
}

// outerTypeParams returns the type parameters of the generic function or
// method whose body declares the local type tn, in source order (receiver
// type parameters first). Go makes a local type distinct for each
// instantiation of its function, so they are implicit type parameters of
// it: its descriptor takes their arguments before its own.
func outerTypeParams(tn *types.TypeName) []*types.TypeParam {
	if tn.Pkg() == nil || tn.Parent() == nil || tn.Parent() == tn.Pkg().Scope() {
		return nil
	}
	if v, ok := outerTPs.Load(tn); ok {
		return v.([]*types.TypeParam)
	}
	var tps []*types.TypeParam
	for s := tn.Parent(); s != nil && s != tn.Pkg().Scope(); s = s.Parent() {
		for _, name := range s.Names() {
			if o, ok := s.Lookup(name).(*types.TypeName); ok {
				if p, ok := o.Type().(*types.TypeParam); ok && p.Obj() == o {
					tps = append(tps, p)
				}
			}
		}
	}
	sort.Slice(tps, func(i, j int) bool { return tps[i].Obj().Pos() < tps[j].Obj().Pos() })
	outerTPs.Store(tn, tps)
	return tps
}

var outerTPs sync.Map // *types.TypeName -> []*types.TypeParam

// hasTypeParam reports whether t mentions a type parameter (or is a local
// type of a generic function, see outerTypeParams).
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
			return len(outerTypeParams(t.Origin().Obj())) > 0
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
	return p.CallBlocksIn(info, call, nil)
}

// CallBlocksIn is CallBlocks for a call in the synchronous clone of assume
// (paramsync.go), or, for a nil assume, anywhere else.
func (p *Program) CallBlocksIn(info *types.Info, call *ast.CallExpr, assume *types.Func) bool {
	u := &unit{encl: p.encls[call]}
	p.classifyCall(info, call, u, false)
	return p.unitBlocksIn(u, p.units, assume)
}

// CallAlwaysAsync reports whether a call that may block is always of an
// async function, which returns a Promise: a call of a declared function or
// method, or a waiting lock. A call of a function value or an interface
// method may be of a function that is not async.
func (p *Program) CallAlwaysAsync(info *types.Info, call *ast.CallExpr) bool {
	if p.pullCalls[call] {
		return false // Pull's next and stop do not switch for a generator
	}
	u := &unit{}
	p.classifyCall(info, call, u, false)
	if u.blocking {
		return true
	}
	for _, c := range u.callees {
		if p.async[c] && !p.syncOnly[c] {
			return true
		}
	}
	for _, s := range u.sites {
		if p.siteBlocks(s, nil) {
			return true
		}
	}
	return false
}

// RangeBlocks reports whether a range-over-func statement's call of its
// iterator may block (and must be awaited).
func (p *Program) RangeBlocks(info *types.Info, s *ast.RangeStmt) bool {
	u := &unit{encl: p.encls[s]}
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

// PureEmitter returns an emitter of the package pkg that only answers
// whether its functions return pure expressions (returnsPure), for the
// lowering of an importer of pkg.
func (p *Program) PureEmitter(pkg *types.Package) *pkgEmitter {
	if pe, ok := p.pureEmitters[pkg]; ok {
		return pe
	}
	if p.pureEmitters == nil {
		p.pureEmitters = map[*types.Package]*pkgEmitter{}
	}
	var pe *pkgEmitter
	if lp := p.byTypes[pkg]; lp != nil {
		pe = newPkgEmitter(p, lp, false)
	}
	p.pureEmitters[pkg] = pe
	return pe
}
