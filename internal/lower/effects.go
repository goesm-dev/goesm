package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
)

// Package initialization without effects. Every package a program imports
// is initialized, so a bundle keeps every package variable initializer and
// every init function of every imported package, with everything they call,
// unless it can tell that they have no effect besides computing their
// variables: net/http's TLS defaults, crypto/tls's cipher suite tables,
// nistec's P-256 table and the Unicode normalization tables of x/text kept
// TLS, X.509 and elliptic curves in a Cloudflare Worker serving one Connect
// service, a quarter of its bundle.
//
// This analysis finds the code that has no effect other than writing memory
// it allocated itself, its local variables, and, for an init function, the
// package-level variables it assigns. A variable initializer without effects
// is declared as a pure expression (LazyVar), which bundlers drop with its
// variable when nothing uses it, and code reachable from the program's roots
// reaches it only when it uses the variable (reach.go); the module leaves
// out one that reachable code does not use (VarReached), which the
// TypeScript of a package run without a bundler would evaluate. An init
// function that only assigns package-level variables runs only if reachable
// code uses one of them (InitCalled). A panic is not an effect here: an
// initializer that would panic is left out with its unused variable, as
// bundlers leave out unused pure code, but an init function that assigns
// nothing and calls panic itself, to check its environment, runs. Effects
// are computed again once the blocking analysis knows what is reachable
// (refineBlocking), since an async function is an effect.
//
// GOESM_EFFDEBUG=1 (or the name of a variable or function, as "var
// net/http.DefaultTransport") prints why initializers have effects.
//
// Each function and function literal gets a summary: whether it may have an
// effect the analysis cannot attribute (impure: a channel operation, a
// goroutine, a call through an interface or a function value, a function
// without a Go body, an async function), the parameters whose memory it
// writes, the package-level variables whose memory it writes, and where the
// memory its results reach comes from. Memory is attributed by where the
// value reaching it comes from, its class: memory the code allocated itself
// (fresh), what a parameter reaches, what a package-level variable reaches,
// or anything else (unknown). A value's class is that of what it is loaded
// from (a field of a parameter's struct reaches what the parameter
// reaches), a composite literal's that of the values in it, a local
// variable's that of every value assigned to it or stored in what it
// reaches, a call's result that of the callee's results in terms of its
// arguments. A package-level variable reaches only its own memory if every
// value the program assigns to it is a new allocation; otherwise it is of
// the unknown class. Storing a value into memory of another class makes the
// memory reach the value's: the store counts as a write of the value's class
// too, since later writes through the memory are attributed to its own.
// Summaries of recursive functions are computed together, to a fixpoint.

type effClass int

const (
	effFresh   effClass = iota // memory the code allocated
	effParam                   // what a parameter reaches
	effGlobal                  // what a package-level variable reaches
	effUnknown                 // anything else
	effLocal                   // a local variable itself (a store target only)
)

type effTarget struct {
	class effClass
	v     *types.Var // the parameter or package-level variable
}

var (
	effFreshT   = effTarget{effFresh, nil}
	effUnknownT = effTarget{effUnknown, nil}
)

// join is the class of a value that may come from a or from b.
func (a effTarget) join(b effTarget) effTarget {
	switch {
	case a.class == effFresh:
		return b
	case b.class == effFresh, a == b:
		return a
	}
	return effUnknownT
}

type effSummary struct {
	impure  bool
	writes  map[*types.Var]bool // parameters whose memory is written
	globals map[*types.Var]bool // package-level variables whose memory is written
	result  effTarget           // what the results reach
	why     string              // what makes it impure, for GOESM_EFFDEBUG
}

func (s *effSummary) equal(o *effSummary) bool {
	if s.impure != o.impure || s.result != o.result || len(s.writes) != len(o.writes) || len(s.globals) != len(o.globals) {
		return false
	}
	for v := range s.writes {
		if !o.writes[v] {
			return false
		}
	}
	for v := range s.globals {
		if !o.globals[v] {
			return false
		}
	}
	return true
}

// record adds a write of the memory of class t.
func (s *effSummary) record(t effTarget, why func() string) {
	switch t.class {
	case effParam:
		if s.writes == nil {
			s.writes = map[*types.Var]bool{}
		}
		s.writes[t.v] = true
	case effGlobal:
		if s.globals == nil {
			s.globals = map[*types.Var]bool{}
		}
		s.globals[t.v] = true
	case effUnknown:
		s.taint(why)
	}
}

func (s *effSummary) taint(why func() string) {
	if !s.impure {
		s.impure = true
		if effDebug != "" {
			s.why = why()
		}
	}
}

var effDebug = os.Getenv("GOESM_EFFDEBUG")

// effNode is a function, a function literal or a package-level
// initializer.
type effNode struct {
	node   ast.Node // the body, or the initializer expression
	info   *types.Info
	params []*types.Var // the receiver first
	top    *effTop
	name   string

	deps    []*effNode // the nodes whose summaries its own depends on
	scanned bool

	index, low int
	onStack    bool
	done       bool
	sum        effSummary
}

// model returns a node for one of sync/atomic's functions, which have no
// Go body: a Load returns what its argument reaches, a Store, Swap or
// CompareAndSwap writes it (and stores the new value in it), an Add or And
// or Or writes it.
func (a *effAnalysis) model(fn *types.Func) *effNode {
	if fn.Pkg() == nil || fn.Pkg().Path() != "sync/atomic" || fn.Signature().Recv() != nil || a.decls[fn] != nil {
		return nil
	}
	if n := a.models[fn]; n != nil {
		return n
	}
	sig := fn.Signature()
	if sig.Params().Len() == 0 {
		return nil
	}
	var params []*types.Var
	for i := 0; i < sig.Params().Len(); i++ {
		params = append(params, sig.Params().At(i))
	}
	n := &effNode{params: params, name: fn.FullName(), scanned: true, done: true, index: -1}
	switch name := fn.Name(); {
	case strings.HasPrefix(name, "Load"):
		n.sum.result = effTarget{effParam, params[0]}
	case strings.HasPrefix(name, "Store"), strings.HasPrefix(name, "Swap"), strings.HasPrefix(name, "CompareAndSwap"),
		strings.HasPrefix(name, "Add"), strings.HasPrefix(name, "And"), strings.HasPrefix(name, "Or"):
		n.sum.writes = map[*types.Var]bool{}
		for _, v := range params {
			if hasPointers(v.Type()) {
				n.sum.writes[v] = true
			}
		}
		n.sum.result = effTarget{effParam, params[0]}
	default:
		return nil
	}
	a.models[fn] = n
	return n
}

// effTop is a declared function or package-level initializer with the
// function literals in it, which share its local variables.
type effTop struct {
	node   ast.Node
	params map[*types.Var]bool
	nodes  []*effNode
}

type effAnalysis struct {
	p      *Program
	decls  map[*types.Func]*ast.FuncDecl
	infos  map[*types.Func]*types.Info
	nodes  map[ast.Node]*effNode
	tops   map[ast.Node]*effTop
	shared map[*types.Var]bool // package-level variables assigned what may reach other memory
	models map[*types.Func]*effNode
	index  int
	stack  []*effNode
}

// effFree are functions without a Go body, or whose natives replace it,
// and interface methods, that have no effect and return memory nobody
// writes.
var effFree = map[string]bool{
	"reflect.TypeOf":                   true,
	"internal/reflectlite.TypeOf":      true,
	"(internal/reflectlite.Type).Elem": true,
	"os.runtime_args":                  true,
	"time.runtimeNano":                 true,
	"errors.New":                       true,
}

// effFreeIface are the interfaces whose methods (but those listed) have no
// effect: reflect.Type's only read the type.
var effFreeIface = map[string]map[string]bool{
	"reflect.Type": {"FieldByNameFunc": true},
}

// findEffects computes lazyVars and lazyInits.
func (p *Program) findEffects() {
	a := &effAnalysis{p: p, decls: map[*types.Func]*ast.FuncDecl{}, infos: map[*types.Func]*types.Info{}, nodes: map[ast.Node]*effNode{}, tops: map[ast.Node]*effTop{}, shared: map[*types.Var]bool{}, models: map[*types.Func]*effNode{}}
	p.lazyVars = map[*types.Var]bool{}
	p.lazyInits = map[*types.Func][]*types.Var{}
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
					if fn, ok := pkg.TypesInfo.Defs[fd.Name].(*types.Func); ok {
						a.decls[fn] = fd
						a.infos[fn] = pkg.TypesInfo
					}
				}
			}
			a.findShared(pkg.TypesInfo, f)
		}
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		embeds := embedDirectives(pkg.Syntax, info)
		for _, in := range info.InitOrder {
			v := initVar(in)
			if v == nil || embeds[v] != nil {
				continue
			}
			n := a.nodeOf(in.Rhs, info, nil, in.Rhs, "var "+v.Pkg().Path()+"."+v.Name())
			a.solve(n)
			if !n.sum.impure && len(n.sum.globals) == 0 && len(n.sum.writes) == 0 {
				p.lazyVars[v] = true
			}
			a.debug(n)
		}
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv != nil || fd.Name.Name != "init" || fd.Body == nil {
					continue
				}
				fn := info.Defs[fd.Name].(*types.Func)
				if p.async[fn] {
					continue
				}
				n := a.funcNode(fn)
				a.solve(n)
				a.debug(n)
				if n.sum.impure || len(n.sum.writes) > 0 || len(n.sum.globals) == 0 && panics(info, fd.Body) {
					continue
				}
				vars := []*types.Var{}
				for v := range n.sum.globals {
					vars = append(vars, v)
				}
				sort.Slice(vars, func(i, j int) bool { return vars[i].Pos() < vars[j].Pos() })
				p.lazyInits[fn] = vars
			}
		}
	}
}

// panics reports whether body calls panic itself, as an init function
// that only checks its environment does (crypto/internal/fips140/check's):
// such a function runs. One that also assigns package variables panics
// only if computing them goes wrong.
func panics(info *types.Info, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if b := builtinOf(info, ast.Unparen(n.Fun)); b != nil && b.Name() == "panic" {
				found = true
			}
		}
		return !found
	})
	return found
}

// debug prints why n has effects, following the calls that have them, when
// GOESM_EFFDEBUG is set (to "1", or to the name of the node).
func (a *effAnalysis) debug(n *effNode) {
	if effDebug == "" || effDebug != "1" && effDebug != n.name {
		return
	}
	var gs []string
	for v := range n.sum.globals {
		gs = append(gs, v.Name())
	}
	sort.Strings(gs)
	fmt.Fprintf(os.Stderr, "effects %s: impure=%v globals=%v\n", n.name, n.sum.impure, gs)
	for i, x := 0, n; x != nil && x.sum.impure && i < 30; i++ {
		fmt.Fprintf(os.Stderr, "\t%s: %s\n", x.name, x.sum.why)
		var next *effNode
		for _, d := range x.deps {
			if d.sum.impure && d.done && d != x {
				if next == nil || x.sum.why == "call of "+d.name {
					next = d
				}
			}
		}
		if next == nil || x.sum.why[:min(len(x.sum.why), 8)] != "call of " {
			break
		}
		x = next
	}
}

// initVar returns the variable initializer in assigns, if it assigns only
// one (var v, _ = f() included), or nil.
func initVar(in *types.Initializer) *types.Var {
	if len(in.Lhs) == 1 {
		return in.Lhs[0]
	}
	var v *types.Var
	for _, l := range in.Lhs {
		if l.Name() != "_" {
			if v != nil {
				return nil
			}
			v = l
		}
	}
	return v
}

// LazyVar reports whether the initializer of package-level variable v has
// no effect besides computing its value (effects.go), so that it is
// declared as a pure expression.
func (p *Program) LazyVar(v *types.Var) bool {
	p.effectsOnce.Do(p.findEffects)
	return p.lazyVars[v]
}

// VarReached reports whether the initializer of package-level variable v
// may run: one without effects runs only if reachable code uses v
// (reach.go); the module leaves the others out.
func (p *Program) VarReached(v *types.Var, init ast.Expr) bool {
	if !p.LazyVar(v) {
		return true
	}
	p.reachOnce.Do(p.reach)
	return p.reachedVarInits[init]
}

// InitCalled reports whether the module calls init function fn: one that
// only assigns package-level variables runs only if reachable code uses one
// of them.
func (p *Program) InitCalled(fn *types.Func) bool {
	p.effectsOnce.Do(p.findEffects)
	if _, ok := p.lazyInits[fn]; !ok {
		return true
	}
	p.reachOnce.Do(p.reach)
	return p.reachedInits[fn]
}

// findShared marks the package-level variables that some code assigns a
// value that may reach memory other than a new allocation.
func (a *effAnalysis) findShared(info *types.Info, f *ast.File) {
	pkgVar := func(e ast.Expr) *types.Var {
		var id *ast.Ident
		switch e := ast.Unparen(e).(type) {
		case *ast.Ident:
			id = e
		case *ast.SelectorExpr:
			if info.Selections[e] == nil {
				id = e.Sel
			}
		}
		if id == nil {
			return nil
		}
		obj := info.Uses[id]
		if obj == nil {
			obj = info.Defs[id]
		}
		if v, ok := obj.(*types.Var); ok && isPkgVar(v) {
			return v
		}
		return nil
	}
	assign := func(lhs []ast.Expr, rhs []ast.Expr) {
		for i, l := range lhs {
			v := pkgVar(l)
			if v == nil || !hasPointers(v.Type()) {
				continue
			}
			if len(rhs) != len(lhs) || !allocExpr(info, rhs[i], v) {
				a.shared[v] = true
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			assign(n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(n.Names))
			for i, id := range n.Names {
				lhs[i] = id
			}
			if len(n.Values) > 0 {
				assign(lhs, n.Values)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if v := pkgVar(e); v != nil && hasPointers(v.Type()) {
					a.shared[v] = true
				}
			}
		}
		return true
	})
}

// allocExpr reports whether e, assigned to package-level variable v, is a
// new allocation that reaches nothing else (or nil, or v grown or
// resliced).
func allocExpr(info *types.Info, e ast.Expr, v *types.Var) bool {
	if !hasPointers(info.TypeOf(e)) {
		return true
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.CompositeLit:
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if _, field := under(info.TypeOf(e)).(*types.Struct); !field && !allocExpr(info, kv.Key, v) {
					return false
				}
				el = kv.Value
			}
			if _, ok := el.(*ast.FuncLit); !ok && !allocExpr(info, el, v) {
				return false
			}
		}
		return true
	case *ast.UnaryExpr:
		lit, ok := ast.Unparen(e.X).(*ast.CompositeLit)
		return e.Op == token.AND && ok && allocExpr(info, lit, v)
	case *ast.Ident:
		return info.Types[e].IsNil() || info.Uses[e] == v
	case *ast.SelectorExpr:
		return info.Uses[e.Sel] == v
	case *ast.SliceExpr:
		return allocExpr(info, e.X, v)
	case *ast.FuncLit:
		return true
	case *ast.CallExpr:
		if tv := info.Types[e.Fun]; tv.IsType() {
			if len(e.Args) != 1 {
				return false
			}
			if b, ok := under(info.TypeOf(e.Args[0])).(*types.Basic); ok && b.Info()&types.IsString != 0 {
				return true // []byte(s), []rune(s)
			}
			return allocExpr(info, e.Args[0], v)
		}
		if b := builtinOf(info, ast.Unparen(e.Fun)); b != nil {
			switch b.Name() {
			case "make", "new":
				return true
			case "append":
				for _, a := range e.Args {
					if !allocExpr(info, a, v) {
						return false
					}
				}
				return true
			}
		}
	}
	return false
}

func isPkgVar(v *types.Var) bool {
	return v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}

// hasPointers reports whether a value of type t may reach memory that code
// holding it can change: through pointers, slices, maps and channels, also
// in fields and elements and behind interfaces.
func hasPointers(t types.Type) bool {
	return hasPointersSeen(t, nil)
}

func hasPointersSeen(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return true
	}
	switch u := under(t).(type) {
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	case *types.Signature:
		return false // calling a function value is impure anyway
	case *types.Struct:
		if seen[t] {
			return false
		}
		if seen == nil {
			seen = map[types.Type]bool{}
		}
		seen[t] = true
		for i := 0; i < u.NumFields(); i++ {
			if hasPointersSeen(u.Field(i).Type(), seen) {
				return true
			}
		}
		return false
	case *types.Array:
		return hasPointersSeen(u.Elem(), seen)
	case *types.Tuple:
		for i := 0; i < u.Len(); i++ {
			if hasPointersSeen(u.At(i).Type(), seen) {
				return true
			}
		}
		return false
	}
	return true
}

func (a *effAnalysis) funcNode(fn *types.Func) *effNode {
	fd := a.decls[fn]
	if fd == nil {
		return nil
	}
	if n := a.nodes[fd.Body]; n != nil {
		return n
	}
	sig := fn.Signature()
	var params []*types.Var
	if sig.Recv() != nil {
		params = append(params, sig.Recv())
	}
	for i := 0; i < sig.Params().Len(); i++ {
		params = append(params, sig.Params().At(i))
	}
	return a.nodeOf(fd.Body, a.infos[fn], params, fd, fn.FullName())
}

// nodeOf returns the node of body (with parameters params) in top.
func (a *effAnalysis) nodeOf(body ast.Node, info *types.Info, params []*types.Var, top ast.Node, name string) *effNode {
	if n := a.nodes[body]; n != nil {
		return n
	}
	t := a.tops[top]
	if t == nil {
		t = &effTop{node: top, params: map[*types.Var]bool{}}
		a.tops[top] = t
		ast.Inspect(top, func(n ast.Node) bool {
			var fl []*ast.FieldList
			switch n := n.(type) {
			case *ast.FuncDecl:
				fl = []*ast.FieldList{n.Recv, n.Type.Params}
			case *ast.FuncLit:
				fl = []*ast.FieldList{n.Type.Params}
			}
			for _, l := range fl {
				if l == nil {
					continue
				}
				for _, f := range l.List {
					for _, id := range f.Names {
						if v, ok := info.Defs[id].(*types.Var); ok {
							t.params[v] = true
						}
					}
				}
			}
			return true
		})
	}
	n := &effNode{node: body, info: info, params: params, top: t, name: name, index: -1}
	a.nodes[body] = n
	t.nodes = append(t.nodes, n)
	return n
}

func (a *effAnalysis) litNode(lit *ast.FuncLit, info *types.Info, top *effTop) *effNode {
	if n := a.nodes[lit.Body]; n != nil {
		return n
	}
	var params []*types.Var
	if sig, ok := info.TypeOf(lit).(*types.Signature); ok {
		for i := 0; i < sig.Params().Len(); i++ {
			params = append(params, sig.Params().At(i))
		}
	}
	pos := a.p.Fset.Position(lit.Pos())
	return a.nodeOf(lit.Body, info, params, top.node, fmt.Sprintf("func literal at %s:%d", pos.Filename, pos.Line))
}

// effFunc is what a call calls: a node, or a function without effects
// (effFree), or nothing the analysis knows.
type effFunc struct {
	node *effNode
	free bool
	fn   *types.Func // a declared function or method
	recv ast.Expr    // the receiver expression of a method value
	sel  *types.Selection
}

// callee finds what call calls.
func (a *effAnalysis) callee(info *types.Info, top *effTop, locals map[*types.Var][]ast.Expr, call *ast.CallExpr) effFunc {
	fun := ast.Unparen(call.Fun)
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = ast.Unparen(f.X)
	case *ast.IndexListExpr:
		fun = ast.Unparen(f.X)
	}
	var c effFunc
	switch f := fun.(type) {
	case *ast.FuncLit:
		c.node = a.litNode(f, info, top)
		return c
	case *ast.Ident:
		switch obj := info.Uses[f].(type) {
		case *types.Func:
			c.fn = obj
		case *types.Var:
			// A local variable only ever assigned one function literal.
			if rhs := locals[obj]; len(rhs) == 1 && rhs[0] != nil {
				if lit, ok := ast.Unparen(rhs[0]).(*ast.FuncLit); ok {
					c.node = a.litNode(lit, info, top)
				}
			}
			return c
		}
	case *ast.SelectorExpr:
		c.sel = info.Selections[f]
		switch {
		case c.sel == nil:
			c.fn, _ = info.Uses[f.Sel].(*types.Func)
		case c.sel.Kind() == types.MethodVal:
			c.fn, _ = c.sel.Obj().(*types.Func)
			c.recv = f.X
		case c.sel.Kind() == types.MethodExpr:
			c.fn, _ = c.sel.Obj().(*types.Func)
		}
	}
	if c.fn == nil {
		return c
	}
	c.fn = c.fn.Origin()
	if effFree[c.fn.FullName()] {
		c.free = true
		return c
	}
	if abstractMethod(c.fn) {
		if c.recv != nil {
			if n, ok := types.Unalias(info.TypeOf(c.recv)).(*types.Named); ok && n.Obj().Pkg() != nil {
				if except, ok := effFreeIface[n.Obj().Pkg().Path()+"."+n.Obj().Name()]; ok && !except[c.fn.Name()] {
					c.free = true
				}
			}
		}
		return c
	}
	if a.p.async[c.fn] {
		return c
	}
	if n := a.model(c.fn); n != nil {
		c.node = n
		return c
	}
	// A function the runtime's natives replace (for speed) behaves as its
	// Go body, if it has one.
	c.node = a.funcNode(c.fn)
	return c
}

// effBody is the state of the analysis of one node.
type effBody struct {
	a      *effAnalysis
	n      *effNode
	info   *types.Info
	top    *effTop
	locals map[*types.Var][]ast.Expr // the values assigned to each local; nil for one not known
	stores map[*types.Var][]ast.Expr // the values stored in what each local reaches
	calls  map[*ast.CallExpr]effFunc // the callees
	// Locals that may reach the same memory (one assigned a value derived
	// from another, such as its address) share a group and a class.
	group map[*types.Var]*types.Var
	cls   map[*types.Var]effTarget // by group
	rets  []ast.Expr               // return values
	sum   effSummary
}

// scan collects the locals of n's top and the callees of n, which are n's
// dependencies with the callees anywhere in its top (whose results the
// locals may hold).
func (a *effAnalysis) scan(n *effNode) *effBody {
	b := &effBody{a: a, n: n, info: n.info, top: n.top, locals: map[*types.Var][]ast.Expr{}, stores: map[*types.Var][]ast.Expr{}, calls: map[*ast.CallExpr]effFunc{}, group: map[*types.Var]*types.Var{}, cls: map[*types.Var]effTarget{}}
	info := n.info
	local := func(e ast.Expr) *types.Var {
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok {
			return nil
		}
		obj := info.Defs[id]
		if obj == nil {
			obj = info.Uses[id]
		}
		v, ok := obj.(*types.Var)
		if !ok || isPkgVar(v) || b.top.params[v] || v.IsField() {
			return nil
		}
		return v
	}
	// base finds the local variable whose memory a store target is in.
	var base func(e ast.Expr) *types.Var
	base = func(e ast.Expr) *types.Var {
		switch e := ast.Unparen(e).(type) {
		case *ast.SelectorExpr:
			if info.Selections[e] != nil {
				if v := local(e.X); v != nil {
					return v
				}
				return base(e.X)
			}
		case *ast.IndexExpr:
			if v := local(e.X); v != nil {
				return v
			}
			return base(e.X)
		case *ast.StarExpr:
			if v := local(e.X); v != nil {
				return v
			}
			return base(e.X)
		}
		return nil
	}
	// mentions joins v's group with those of the locals e mentions.
	mentions := func(v *types.Var, e ast.Expr) {
		if e == nil || !hasPointers(v.Type()) {
			return
		}
		ast.Inspect(e, func(x ast.Node) bool {
			if id, ok := x.(*ast.Ident); ok {
				if u := local(id); u != nil && u != v && hasPointers(u.Type()) {
					b.union(v, u)
				}
			}
			return true
		})
	}
	assign := func(lhs []ast.Expr, rhs []ast.Expr) {
		for i, l := range lhs {
			var r ast.Expr
			if len(rhs) == len(lhs) {
				r = rhs[i]
			} else if len(rhs) == 1 {
				if call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr); ok {
					r = call // one of several results
				}
			}
			if v := local(l); v != nil {
				b.locals[v] = append(b.locals[v], r)
				mentions(v, r)
				continue
			}
			if v := base(l); v != nil {
				b.stores[v] = append(b.stores[v], r)
				mentions(v, r)
			}
		}
	}
	ast.Inspect(n.top.node, func(x ast.Node) bool {
		switch x := x.(type) {
		case *ast.AssignStmt:
			assign(x.Lhs, x.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(x.Names))
			for i, id := range x.Names {
				lhs[i] = id
			}
			if len(x.Values) == 0 {
				for _, l := range lhs {
					if v := local(l); v != nil {
						b.locals[v] = append(b.locals[v], ast.NewIdent("nil"))
					}
				}
			} else {
				assign(lhs, x.Values)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{x.Key, x.Value} {
				if e == nil {
					continue
				}
				if v := local(e); v != nil {
					// An element of what is ranged over.
					b.locals[v] = append(b.locals[v], x.X)
					mentions(v, x.X)
				}
			}
		case *ast.TypeSwitchStmt:
			var src ast.Expr
			if as, ok := x.Assign.(*ast.AssignStmt); ok && len(as.Rhs) == 1 {
				if ta, ok := as.Rhs[0].(*ast.TypeAssertExpr); ok {
					src = ta.X
				}
			}
			for _, c := range x.Body.List {
				if v, ok := info.Implicits[c].(*types.Var); ok {
					b.locals[v] = append(b.locals[v], src)
					mentions(v, src)
				}
			}
		case *ast.CallExpr:
			c := a.callee(info, n.top, b.locals, x)
			b.calls[x] = c
		}
		return true
	})
	// The callees whose function literal locals were assigned after the
	// calls were seen.
	for call := range b.calls {
		b.calls[call] = a.callee(info, n.top, b.locals, call)
	}
	if !n.scanned {
		n.scanned = true
		seen := map[*effNode]bool{}
		for _, c := range b.calls {
			if c.node != nil && c.node.node != nil && !seen[c.node] { // not a model
				seen[c.node] = true
				n.deps = append(n.deps, c.node)
			}
		}
		sort.Slice(n.deps, func(i, j int) bool { return n.deps[i].node.Pos() < n.deps[j].node.Pos() })
	}
	return b
}

func (b *effBody) find(v *types.Var) *types.Var {
	for {
		g, ok := b.group[v]
		if !ok || g == v {
			return v
		}
		v = g
	}
}

func (b *effBody) union(u, v *types.Var) {
	gu, gv := b.find(u), b.find(v)
	if gu != gv {
		b.group[gu] = gv
	}
}

// classes computes the class of each group of locals, to a fixpoint.
func (b *effBody) classes() {
	vals := func(v *types.Var, es []ast.Expr, assigned bool) {
		g := b.find(v)
		c := b.cls[g]
		for _, e := range es {
			if e == nil {
				c = effUnknownT
				break
			}
			if id, ok := e.(*ast.Ident); ok && id.Name == "nil" && id.Pos() == token.NoPos {
				continue // var v T
			}
			if assigned || hasPointers(b.info.TypeOf(e)) {
				c = c.join(b.class(e))
			}
		}
		b.cls[g] = c
	}
	for {
		old := len(b.cls)
		before := map[*types.Var]effTarget{}
		for g, c := range b.cls {
			before[g] = c
		}
		for v, rhs := range b.locals {
			if hasPointers(v.Type()) {
				vals(v, rhs, true)
			}
		}
		for v, rhs := range b.stores {
			if hasPointers(v.Type()) {
				vals(v, rhs, false)
			}
		}
		same := len(b.cls) == old
		for g, c := range b.cls {
			if before[g] != c {
				same = false
			}
		}
		if same {
			return
		}
	}
}

// class is the class of what the value of e reaches.
func (b *effBody) class(e ast.Expr) effTarget {
	info := b.info
	if tv, ok := info.Types[e]; ok && (tv.Value != nil || tv.IsNil()) {
		return effFreshT
	}
	if t := info.TypeOf(e); t != nil && !hasPointers(t) {
		if _, ok := t.(*types.Tuple); !ok {
			return effFreshT
		}
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		switch obj := info.Uses[e].(type) {
		case *types.Var:
			switch {
			case isPkgVar(obj):
				if b.a.shared[obj] {
					return effUnknownT
				}
				return effTarget{effGlobal, obj}
			case b.top.params[obj]:
				return effTarget{effParam, obj}
			}
			return b.cls[b.find(obj)]
		case *types.Func, *types.Nil:
			return effFreshT
		}
	case *ast.SelectorExpr:
		if sel := info.Selections[e]; sel != nil {
			if sel.Kind() == types.FieldVal {
				return b.class(e.X)
			}
			return effFreshT // a method value: calling it is impure
		}
		switch obj := info.Uses[e.Sel].(type) {
		case *types.Var:
			if isPkgVar(obj) && !b.a.shared[obj] {
				return effTarget{effGlobal, obj}
			}
		case *types.Func:
			return effFreshT
		}
	case *ast.IndexExpr:
		if tv := info.Types[e.X]; tv.IsValue() {
			return b.class(e.X)
		}
		return effFreshT // an instantiated function
	case *ast.IndexListExpr:
		return effFreshT
	case *ast.StarExpr:
		return b.class(e.X)
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return b.class(e.X)
		}
	case *ast.SliceExpr:
		return b.class(e.X)
	case *ast.TypeAssertExpr:
		return b.class(e.X)
	case *ast.CompositeLit:
		c := effFreshT
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if _, field := under(info.TypeOf(e)).(*types.Struct); !field {
					c = c.join(b.class(kv.Key))
				}
				el = kv.Value
			}
			c = c.join(b.class(el))
		}
		return c
	case *ast.FuncLit:
		return effFreshT
	case *ast.BinaryExpr:
		return effFreshT
	case *ast.CallExpr:
		if tv := info.Types[e.Fun]; tv.IsType() {
			if len(e.Args) != 1 {
				return effUnknownT
			}
			if bt, ok := under(info.TypeOf(e.Args[0])).(*types.Basic); ok && bt.Info()&types.IsString != 0 {
				return effFreshT // []byte(s), []rune(s)
			}
			return b.class(e.Args[0])
		}
		if bi := builtinOf(info, ast.Unparen(e.Fun)); bi != nil {
			switch bi.Name() {
			case "make", "new":
				return effFreshT
			case "append":
				c := effFreshT
				for _, a := range e.Args {
					c = c.join(b.class(a))
				}
				return c
			}
			return effUnknownT
		}
		return b.resultClass(e)
	}
	return effUnknownT
}

// resultClass is the class of what the results of call reach.
func (b *effBody) resultClass(call *ast.CallExpr) effTarget {
	c, ok := b.calls[call]
	switch {
	case !ok:
		return effUnknownT
	case c.free:
		return effFreshT
	case c.node == nil:
		return effUnknownT
	}
	s := &c.node.sum
	if s.impure {
		return effUnknownT
	}
	r := s.result
	if r.class == effParam {
		args := b.args(call, c)
		for i, p := range c.node.params {
			if p == r.v {
				if i < len(args) {
					return args[i]
				}
				return effUnknownT
			}
		}
		// A parameter of an enclosing function that a function literal
		// returns what it reaches.
		return r
	}
	return r
}

// loc is the class of the storage that the store target e denotes.
func (b *effBody) loc(e ast.Expr) effTarget {
	info := b.info
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		obj := info.Uses[e]
		if obj == nil {
			obj = info.Defs[e]
		}
		v, ok := obj.(*types.Var)
		if !ok {
			return effUnknownT
		}
		if isPkgVar(v) {
			return effTarget{effGlobal, v}
		}
		return effTarget{effLocal, v}
	case *ast.SelectorExpr:
		sel := info.Selections[e]
		if sel == nil {
			if v, ok := info.Uses[e.Sel].(*types.Var); ok && isPkgVar(v) {
				return effTarget{effGlobal, v}
			}
			return effUnknownT
		}
		if sel.Kind() != types.FieldVal {
			return effUnknownT
		}
		if _, ok := under(info.TypeOf(e.X)).(*types.Pointer); ok || len(sel.Index()) > 1 && embeddedPointer(info.TypeOf(e.X), sel.Index()) {
			return b.class(e.X)
		}
		return b.loc(e.X)
	case *ast.IndexExpr:
		switch under(info.TypeOf(e.X)).(type) {
		case *types.Array:
			return b.loc(e.X)
		case *types.Pointer, *types.Slice, *types.Map:
			return b.class(e.X)
		}
	case *ast.StarExpr:
		return b.class(e.X)
	case *ast.CompositeLit:
		return effFreshT
	}
	return effUnknownT
}

// store accounts for assigning the value e (nil if not known) to the store
// target lhs.
func (b *effBody) store(lhs ast.Expr, e ast.Expr) {
	if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
		return
	}
	t := b.loc(lhs)
	pos := lhs.Pos()
	if t.class != effLocal {
		b.sum.record(t, func() string { return "write at " + b.where(pos) })
	}
	if e != nil && !hasPointers(b.info.TypeOf(e)) {
		return
	}
	var c effTarget
	if e == nil {
		c = effUnknownT
	} else {
		c = b.class(e)
	}
	if t.class == effLocal || c.class == effFresh || c == t {
		return
	}
	// The memory written now reaches what e reaches: later writes through
	// it count as writes of its own class.
	if t.class == effFresh {
		return // a local's memory, whose class the stores join (classes)
	}
	b.sum.record(c, func() string { return "store of untracked memory at " + b.where(pos) })
}

func (b *effBody) where(pos token.Pos) string {
	p := b.a.p.Fset.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}

// args returns the classes of what the arguments of call reach, by callee
// parameter, the receiver first.
func (b *effBody) args(call *ast.CallExpr, c effFunc) []effTarget {
	info := b.info
	var args []effTarget
	exprs := call.Args
	var sig *types.Signature
	if c.fn != nil {
		sig = c.fn.Signature()
		if sig.Recv() != nil {
			switch {
			case c.recv != nil:
				args = append(args, b.recvClass(c.recv, c.sel, c.fn))
			case len(exprs) > 0:
				args = append(args, b.class(exprs[0]))
				exprs = exprs[1:]
			default:
				args = append(args, effUnknownT)
			}
		}
	} else {
		sig, _ = under(info.TypeOf(call.Fun)).(*types.Signature)
		if lit, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
			sig, _ = info.TypeOf(lit).(*types.Signature)
		}
	}
	if sig == nil {
		return nil
	}
	params := sig.Params()
	if len(exprs) == 1 && params.Len() > 1 {
		// f(g()): each argument reaches what g's results reach.
		r := b.class(exprs[0])
		for i := 0; i < params.Len(); i++ {
			args = append(args, r)
		}
		return args
	}
	for i := 0; i < params.Len(); i++ {
		switch {
		case sig.Variadic() && i == params.Len()-1:
			if call.Ellipsis.IsValid() && i < len(exprs) {
				args = append(args, b.class(exprs[i]))
				break
			}
			c := effFreshT // a new slice of the arguments
			for _, e := range exprs[min(i, len(exprs)):] {
				c = c.join(b.class(e))
			}
			args = append(args, c)
		case i < len(exprs):
			args = append(args, b.class(exprs[i]))
		default:
			args = append(args, effUnknownT)
		}
	}
	return args
}

// recvClass is the class of what the receiver of a method call x.M()
// reaches.
func (b *effBody) recvClass(x ast.Expr, sel *types.Selection, fn *types.Func) effTarget {
	if _, ptrRecv := types.Unalias(fn.Signature().Recv().Type()).(*types.Pointer); ptrRecv {
		if _, ptrX := under(b.info.TypeOf(x)).(*types.Pointer); !ptrX && (len(sel.Index()) == 1 || !embeddedPointer(b.info.TypeOf(x), sel.Index())) {
			// &x: a write through the receiver writes x.
			l := b.loc(x)
			if l.class == effLocal {
				return b.class(x).join(effFreshT)
			}
			return l
		}
	}
	return b.class(x)
}

// analyze computes n's summary from its statements and the current
// summaries of its callees.
func (a *effAnalysis) analyze(n *effNode) effSummary {
	b := a.scan(n)
	b.classes()
	info := n.info
	s := &b.sum
	ast.Inspect(n.node, func(x ast.Node) bool {
		if s.impure {
			return false
		}
		switch x := x.(type) {
		case *ast.FuncLit:
			return false // a separate node, analyzed where it is called
		case *ast.AssignStmt:
			for i, l := range x.Lhs {
				if id, ok := l.(*ast.Ident); ok && x.Tok == token.DEFINE && info.Defs[id] != nil {
					continue // a new local
				}
				var r ast.Expr
				if len(x.Rhs) == len(x.Lhs) {
					r = x.Rhs[i]
				} else if call, ok := ast.Unparen(x.Rhs[0]).(*ast.CallExpr); ok {
					r = call
				}
				if x.Tok != token.ASSIGN && x.Tok != token.DEFINE {
					r = nil
					if !hasPointers(info.TypeOf(l)) {
						r = l
					}
				}
				b.store(l, r)
			}
		case *ast.IncDecStmt:
			b.store(x.X, x.X)
		case *ast.RangeStmt:
			switch under(info.TypeOf(x.X)).(type) {
			case *types.Chan, *types.Signature:
				pos := x.Pos()
				s.taint(func() string { return "range over a channel or function at " + b.where(pos) })
			}
			if x.Tok == token.ASSIGN {
				for _, l := range []ast.Expr{x.Key, x.Value} {
					if l != nil {
						b.store(l, x.X)
					}
				}
			}
		case *ast.SendStmt, *ast.GoStmt, *ast.SelectStmt:
			pos := x.Pos()
			s.taint(func() string { return "channel operation or goroutine at " + b.where(pos) })
		case *ast.UnaryExpr:
			if x.Op == token.ARROW {
				pos := x.Pos()
				s.taint(func() string { return "channel receive at " + b.where(pos) })
			}
		case *ast.ReturnStmt:
			b.rets = append(b.rets, x.Results...)
		case *ast.CallExpr:
			b.call(x)
		}
		return true
	})
	if !s.impure {
		r := effFreshT
		for _, e := range b.rets {
			if len(b.rets) > 0 && hasPointers(info.TypeOf(e)) {
				r = r.join(b.class(e))
			}
		}
		if fd, ok := n.top.node.(*ast.FuncDecl); ok && fd.Body == n.node && fd.Type.Results != nil {
			for _, f := range fd.Type.Results.List {
				for _, id := range f.Names {
					if v, ok := info.Defs[id].(*types.Var); ok && hasPointers(v.Type()) {
						r = r.join(b.cls[b.find(v)])
					}
				}
			}
		}
		if _, ok := n.node.(ast.Expr); ok {
			r = b.class(n.node.(ast.Expr))
		}
		s.result = r
	}
	return *s
}

func (b *effBody) call(call *ast.CallExpr) {
	info := b.info
	s := &b.sum
	fun := ast.Unparen(call.Fun)
	if tv, ok := info.Types[fun]; ok && tv.IsType() {
		return // a conversion
	}
	pos := call.Pos()
	if bi := builtinOf(info, fun); bi != nil {
		switch bi.Name() {
		case "append":
			// Writes the elements after the length of the first argument.
			b.storeElems(call.Args[0], call.Args[1:], call.Ellipsis.IsValid(), pos)
		case "copy":
			b.storeElems(call.Args[0], call.Args[1:], true, pos)
		case "clear", "delete":
			b.sum.record(b.class(call.Args[0]), func() string { return "clear or delete at " + b.where(pos) })
		case "close", "print", "println":
			s.taint(func() string { return bi.Name() + " at " + b.where(pos) })
		}
		return
	}
	c := b.calls[call]
	if c.free {
		return
	}
	if c.node == nil {
		s.taint(func() string {
			if c.fn != nil {
				return "call of " + c.fn.FullName() + " (no Go body, async or through an interface) at " + b.where(pos)
			}
			return "call through a function value at " + b.where(pos)
		})
		return
	}
	cs := &c.node.sum
	if cs.impure {
		s.taint(func() string { return "call of " + c.node.name })
		return
	}
	for v := range cs.globals {
		s.record(effTarget{effGlobal, v}, nil)
	}
	if len(cs.writes) == 0 {
		return
	}
	args := b.args(call, c)
	for v := range cs.writes {
		i := -1
		for j, p := range c.node.params {
			if p == v {
				i = j
				break
			}
		}
		switch {
		case i < 0:
			// What a parameter of an enclosing function reaches, which a
			// function literal writes.
			s.record(effTarget{effParam, v}, nil)
		case i < len(args):
			s.record(args[i], func() string { return "call of " + c.node.name + " writing untracked memory at " + b.where(pos) })
		default:
			s.taint(func() string { return "call of " + c.node.name + " at " + b.where(pos) })
		}
	}
}

// storeElems accounts for append or copy writing the elements elems (one
// slice if spread) into the memory of dst.
func (b *effBody) storeElems(dst ast.Expr, elems []ast.Expr, spread bool, pos token.Pos) {
	t := b.class(dst)
	b.sum.record(t, func() string { return "append or copy at " + b.where(pos) })
	for _, e := range elems {
		et := b.info.TypeOf(e)
		if spread {
			if s, ok := under(et).(*types.Slice); ok {
				et = s.Elem()
			} else {
				continue // copy from a string
			}
		}
		if !hasPointers(et) {
			continue
		}
		c := b.class(e)
		if c.class != effFresh && c != t && t.class != effFresh {
			b.sum.record(c, func() string { return "store of untracked memory at " + b.where(pos) })
		}
	}
}

// embeddedPointer reports whether the path of embedded fields to a
// promoted field or method goes through a pointer after its start.
func embeddedPointer(t types.Type, index []int) bool {
	if p, ok := under(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	for _, i := range index[:len(index)-1] {
		s, ok := under(t).(*types.Struct)
		if !ok {
			return true
		}
		t = s.Field(i).Type()
		if _, ok := under(t).(*types.Pointer); ok {
			return true
		}
	}
	return false
}

func builtinOf(info *types.Info, e ast.Expr) *types.Builtin {
	switch e := e.(type) {
	case *ast.Ident:
		b, _ := info.Uses[e].(*types.Builtin)
		return b
	case *ast.SelectorExpr:
		b, _ := info.Uses[e.Sel].(*types.Builtin)
		return b
	}
	return nil
}

// solve computes the summary of n and of every node it depends on
// (Tarjan's strongly connected components, each solved to a fixpoint).
func (a *effAnalysis) solve(n *effNode) {
	if !n.done {
		a.connect(n)
	}
}

func (a *effAnalysis) connect(n *effNode) {
	if !n.scanned {
		a.scan(n)
	}
	n.index, n.low = a.index, a.index
	a.index++
	a.stack = append(a.stack, n)
	n.onStack = true
	for _, w := range n.deps {
		switch {
		case w.done:
		case w.index < 0:
			a.connect(w)
			n.low = min(n.low, w.low)
		case w.onStack:
			n.low = min(n.low, w.index)
		}
	}
	if n.low != n.index {
		return
	}
	var scc []*effNode
	for {
		w := a.stack[len(a.stack)-1]
		a.stack = a.stack[:len(a.stack)-1]
		w.onStack = false
		scc = append(scc, w)
		if w == n {
			break
		}
	}
	for changed := true; changed; {
		changed = false
		for _, w := range scc {
			s := a.analyze(w)
			s.join(&w.sum)
			if !s.equal(&w.sum) {
				w.sum = s
				changed = true
			}
		}
	}
	for _, w := range scc {
		w.done = true
	}
}

// join adds o's effects to s, so that summaries only grow while an SCC is
// solved.
func (s *effSummary) join(o *effSummary) {
	if o.impure && !s.impure {
		s.impure, s.why = true, o.why
	}
	for v := range o.writes {
		s.record(effTarget{effParam, v}, nil)
	}
	for v := range o.globals {
		s.record(effTarget{effGlobal, v}, nil)
	}
	s.result = s.result.join(o.result)
}
