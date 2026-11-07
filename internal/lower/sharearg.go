package lower

import (
	"go/ast"
	"go/token"
	"go/types"
	"sync"
)

// Struct and array arguments passed without a copy.
//
// A struct or array is a JS object, and passing one by value copies it
// ($clone, $rt.copy) so that the callee gets a value of its own. When the
// callee only reads its parameter, the copy cannot be told apart from the
// caller's value as long as nothing writes the caller's value while the
// call runs, and the copy is an allocation per call that a hand-written
// TypeScript function, passing its object, does not make: small helpers
// that take a Point and pass it on to one another allocate a copy at every
// call.
//
// An argument is passed without a copy when the call is a direct call of a
// declared synchronous function, not deferred or started as a goroutine,
// whose parameter is never modified, addressed, sliced or referenced by a
// function literal in its body (mutatesVar, as for value receivers), and
// either
//   - the argument is a local variable whose address is never taken and
//     that no function literal references, or a field or array element of
//     one: nothing else can reach it, so nothing writes it during the call;
//   - or the callee writes no memory but what it allocates and its locals
//     (effects.go), so nothing it does can change the value.
//
// The callee may pass the parameter on uncopied by the same rules: it is
// one of its local variables, and its callees run within it.

type shareArgs struct {
	mu       sync.Mutex
	calls    map[*ast.CallExpr]uint64 // bit i: argument i is passed as it is
	readOnly map[*types.Var]bool      // parameters the callee only reads
	captured map[*types.Var]bool      // variables function literals reference
	deferred map[*ast.CallExpr]bool   // calls of go and defer statements
	scanned  map[*types.Info]bool     // the packages captured and deferred cover
	free     map[*types.Func]bool     // callees that write no memory
}

// ShareArgs returns the arguments of call (bit i for argument i) that are
// passed without a copy.
func (p *Program) ShareArgs(info *types.Info, call *ast.CallExpr) uint64 {
	s := &p.shareArgs
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.calls[call]; ok {
		return m
	}
	s.init()
	m := p.shareArgsOf(info, call)
	s.calls[call] = m
	return m
}

func (s *shareArgs) init() {
	if s.calls == nil {
		s.calls, s.readOnly, s.captured, s.deferred, s.scanned, s.free = map[*ast.CallExpr]uint64{}, map[*types.Var]bool{}, map[*types.Var]bool{}, map[*ast.CallExpr]bool{}, map[*types.Info]bool{}, map[*types.Func]bool{}
	}
}

// ReturnsLocal reports whether the result expression e of a return
// statement is a local variable that no pointer, closure, go or defer
// statement can reach (privateValue): the function returns the variable's
// value without a copy, as the variable is not used after the return. The
// caller makes sure that no other result is the same variable and that it
// is not a parameter, which the caller may have passed without a copy
// (ShareArgs).
func (p *Program) ReturnsLocal(info *types.Info, e ast.Expr) bool {
	id, ok := unparen(e).(*ast.Ident)
	if !ok || p.eff == nil {
		return false
	}
	s := &p.shareArgs
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	p.scanShare(info)
	return p.privateValue(info, id)
}

// PrivateVar reports whether the local variable v is one that no pointer,
// closure, go or defer statement can reach (see privateValue).
func (p *Program) PrivateVar(info *types.Info, v *types.Var) bool {
	if p.eff == nil || v.IsField() || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
		return false
	}
	s := &p.shareArgs
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	p.scanShare(info)
	return !p.boxed[v] && !s.captured[v]
}

func (p *Program) shareArgsOf(info *types.Info, call *ast.CallExpr) uint64 {
	if p.eff == nil || len(call.Args) > 64 || call.Ellipsis.IsValid() {
		return 0
	}
	fn := staticCallee(info, call)
	if fn == nil || p.async[fn] {
		return 0
	}
	fd := p.eff.decls[fn]
	if fd == nil || fd.Body == nil {
		return 0
	}
	p.scanShare(info)
	if p.shareArgs.deferred[call] {
		return 0
	}
	sig := fn.Signature()
	params := sig.Params()
	var mask uint64
	for i, a := range call.Args {
		if i >= params.Len() || sig.Variadic() && i >= params.Len()-1 {
			break
		}
		pv := params.At(i)
		if !isAggregate(pv.Type()) || isTypeParam(pv.Type()) {
			continue
		}
		if _, isTuple := info.TypeOf(a).(*types.Tuple); isTuple {
			return 0
		}
		if !p.paramReadOnly(fn, fd, pv) {
			continue
		}
		if p.privateValue(info, a) || p.writesNothing(fn) {
			mask |= 1 << i
		}
	}
	return mask
}

// paramReadOnly reports whether fd, declaring fn, never modifies or keeps a
// reference to its parameter pv.
func (p *Program) paramReadOnly(fn *types.Func, fd *ast.FuncDecl, pv *types.Var) bool {
	s := &p.shareArgs
	if ro, ok := s.readOnly[pv]; ok {
		return ro
	}
	info := p.eff.infos[fn]
	ro := info != nil && !p.boxed[pv] && !mutatesVarIn(info, fd.Body, pv)
	s.readOnly[pv] = ro
	return ro
}

// privateValue reports whether e is a local variable that only its own
// function's code (not its function literals) reaches, or a field or array
// element of one.
func (p *Program) privateValue(info *types.Info, e ast.Expr) bool {
	for {
		switch x := unparen(e).(type) {
		case *ast.Ident:
			v, ok := info.Uses[x].(*types.Var)
			if !ok || v.IsField() || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
				return false
			}
			return !p.boxed[v] && !p.shareArgs.captured[v]
		case *ast.SelectorExpr:
			if sel, ok := info.Selections[x]; !ok || sel.Kind() != types.FieldVal || sel.Indirect() {
				return false
			}
			if _, isPtr := under(info.TypeOf(x.X)).(*types.Pointer); isPtr {
				return false
			}
			e = x.X
		case *ast.IndexExpr:
			if _, ok := under(info.TypeOf(x.X)).(*types.Array); !ok {
				return false
			}
			e = x.X
		default:
			return false
		}
	}
}

// writesNothing reports whether fn writes no memory but what it allocates
// and its local variables.
func (p *Program) writesNothing(fn *types.Func) bool {
	s := &p.shareArgs
	if f, ok := s.free[fn]; ok {
		return f
	}
	n := p.eff.funcNode(fn)
	free := false
	if n != nil {
		p.eff.solve(n)
		free = !n.sum.impure && len(n.sum.writes) == 0 && len(n.sum.globals) == 0
	}
	s.free[fn] = free
	return free
}

// scanShare records which variables of the package of info function
// literals reference, and its calls in go and defer statements.
func (p *Program) scanShare(info *types.Info) {
	s := &p.shareArgs
	if s.scanned[info] {
		return
	}
	s.scanned[info] = true
	for _, pkg := range p.Pkgs {
		if pkg.TypesInfo != info {
			continue
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.GoStmt:
					s.deferred[n.Call] = true
				case *ast.DeferStmt:
					s.deferred[n.Call] = true
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						s.markRoot(info, n.X)
					}
				case *ast.SliceExpr:
					s.markRoot(info, n.X)
				case *ast.SelectorExpr:
					if sel, ok := info.Selections[n]; ok && sel.Kind() == types.MethodVal {
						if _, ptr := sel.Obj().(*types.Func).Signature().Recv().Type().(*types.Pointer); ptr {
							s.markRoot(info, n.X)
						}
					}
				case *ast.FuncLit:
					ast.Inspect(n.Body, func(m ast.Node) bool {
						if id, ok := m.(*ast.Ident); ok {
							if v, ok := info.Uses[id].(*types.Var); ok && !v.IsField() && (v.Pos() < n.Pos() || v.Pos() >= n.End()) {
								s.captured[v] = true
							}
						}
						return true
					})
				}
				return true
			})
		}
	}
}

// markRoot marks the variable whose memory e is (a variable, or a field or
// array element of one) as reachable from elsewhere: its address is taken,
// it is sliced, or a pointer method is called on it. For a struct or array
// variable, which is its JS object, p.boxed does not tell.
func (s *shareArgs) markRoot(info *types.Info, e ast.Expr) {
	for {
		switch x := unparen(e).(type) {
		case *ast.Ident:
			if v, ok := info.Uses[x].(*types.Var); ok {
				s.captured[v] = true
			}
			return
		case *ast.SelectorExpr:
			if _, isPtr := under(info.TypeOf(x.X)).(*types.Pointer); isPtr {
				return
			}
			e = x.X
		case *ast.IndexExpr:
			if _, ok := under(info.TypeOf(x.X)).(*types.Array); !ok {
				return
			}
			e = x.X
		default:
			return
		}
	}
}

// mutatesVarIn is funcEmitter.mutatesVar for any package's body.
func mutatesVarIn(info *types.Info, body *ast.BlockStmt, v *types.Var) bool {
	return (&funcEmitter{info: info}).mutatesVar(body, v)
}
