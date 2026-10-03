package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Mutexes held across blocking operations.
//
// goesm's sync.Mutex.Lock never waits (see internal/natives/goroot/sync): a
// goroutine can only find a mutex locked if another goroutine holds it while
// blocked. This file finds the Lock calls that may have to wait and lowers
// them to the waiting (async) lockSlow, rLockSlow or, through a sync.Locker,
// lockerSlow.
//
// A Lock or RLock call statement opens a critical section that lasts until
// an Unlock or RUnlock of the same expression later in the same statement
// list, or to the end of the list with a deferred unlock. The section holds
// the mutex across a blocking operation if it may block, or if it has no
// unlock at all (the mutex stays locked when the function returns, as in a
// lock() helper method).
//
// A mutex is named by the variable or struct field that holds it ("direct",
// mu.Lock() or s.mu.Lock()). It may also be reached indirectly: through a
// *sync.Mutex variable or parameter, an expression without a name, or a
// sync.Locker. Since names are not alias-free, the rules are:
//
//   - H: direct mutexes held across a blocking operation;
//   - U: some mutex reached indirectly is held across a blocking operation;
//   - E: direct mutexes whose address escapes (&mu, a method value mu.Lock,
//     sync.NewCond(&mu), rw.RLocker(), or a struct embedding the mutex stored
//     in an interface value, whose promoted Lock can then be called through
//     sync.Locker).
//
// A direct Lock of m waits if m is in H, or U holds and m is in E. An
// indirect Lock waits if U holds or a mutex in H is also in E. Waiting Locks
// are async, which can make more sections block, so this repeats with the
// blocking analysis until nothing changes.
//
// lockerSlow cannot see through a sync.Locker whose dynamic type is a struct
// embedding the mutex; it retries such a Lock after each unlock. Package
// sync's own locking (Cond.Wait, RLocker) is left out of the analysis.

// mutexCall is a call of a sync.Mutex or sync.RWMutex method, or of
// sync.Locker's.
type mutexCall struct {
	call   *ast.CallExpr
	method string       // Lock, RLock, Unlock, ...
	key    types.Object // the variable or field locked, if direct
	expr   string       // the receiver expression
	slow   *types.Func  // the waiting variant of Lock and RLock
	locker bool         // a sync.Locker method call
}

func (mc mutexCall) direct() bool { return mc.key != nil }

func isLock(method string) bool { return method == "Lock" || method == "RLock" }

func isUnlock(method string) bool { return method == "Unlock" || method == "RUnlock" }

// syncMutexType reports whether t is sync.Mutex or sync.RWMutex.
func syncMutexType(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "sync" {
		return false
	}
	return named.Obj().Name() == "Mutex" || named.Obj().Name() == "RWMutex"
}

// slowMethod returns the waiting variant of method on sync type t (a
// pointer to Mutex or RWMutex, or the waiter type for sync.Locker).
func slowMethod(t types.Type, pkg *types.Package, method string) *types.Func {
	name := map[string]string{"Lock": "lockSlow", "RLock": "rLockSlow"}[method]
	if name == "" {
		return nil
	}
	obj, _, _ := types.LookupFieldOrMethod(t, false, pkg, name)
	fn, _ := obj.(*types.Func)
	return fn
}

// mutexMethod recognizes a call of a sync.Mutex, sync.RWMutex or
// sync.Locker method.
func mutexMethod(info *types.Info, call *ast.CallExpr) (mutexCall, bool) {
	sel, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return mutexCall{}, false
	}
	s, ok := info.Selections[sel]
	if !ok || (s.Kind() != types.MethodVal && s.Kind() != types.MethodExpr) {
		return mutexCall{}, false
	}
	fn := s.Obj().(*types.Func)
	if fn.Pkg() == nil || fn.Pkg().Path() != "sync" {
		return mutexCall{}, false
	}
	x := sel.X
	if s.Kind() == types.MethodExpr {
		// (*sync.Mutex).Lock(&mu) locks what mu.Lock() does. A method
		// promoted from an embedded mutex is left out.
		if len(call.Args) == 0 || len(s.Index()) > 1 {
			return mutexCall{}, false
		}
		x = call.Args[0]
		if u, ok := unparen(x).(*ast.UnaryExpr); ok && u.Op == token.AND {
			x = u.X
		}
	}
	mc := mutexCall{call: call, method: fn.Name(), expr: types.ExprString(x)}
	recv := fn.Signature().Recv().Type()
	if named, ok := types.Unalias(recv).(*types.Named); ok && named.Obj().Name() == "Locker" {
		mc.locker = true
		if mc.method == "Lock" {
			if w := fn.Pkg().Scope().Lookup("waiter"); w != nil {
				obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(w.Type()), false, fn.Pkg(), "lockerSlow")
				mc.slow, _ = obj.(*types.Func)
			}
		}
		return mc, true
	}
	ptr, ok := recv.(*types.Pointer)
	if !ok || !syncMutexType(ptr.Elem()) {
		return mutexCall{}, false
	}
	mc.slow = slowMethod(ptr, fn.Pkg(), fn.Name())
	if path := s.Index(); len(path) > 1 {
		// Promoted from an embedded field: the mutex is the last one.
		t := info.TypeOf(sel.X)
		var f *types.Var
		for _, idx := range path[:len(path)-1] {
			base, _ := derefType(t)
			f = base.Underlying().(*types.Struct).Field(idx)
			t = f.Type()
		}
		if syncMutexType(f.Type()) {
			mc.key = f.Origin()
		}
		return mc, true
	}
	mc.key = mutexKey(info, x)
	return mc, true
}

// mutexKey returns the variable or field that x (an expression of type
// sync.Mutex or sync.RWMutex) names, or nil.
func mutexKey(info *types.Info, x ast.Expr) types.Object {
	if !syncMutexType(info.TypeOf(x)) {
		return nil // a pointer: the mutex has no name here
	}
	switch x := unparen(x).(type) {
	case *ast.Ident:
		if v, ok := info.ObjectOf(x).(*types.Var); ok {
			return v.Origin()
		}
	case *ast.SelectorExpr:
		if fs, ok := info.Selections[x]; ok && fs.Kind() == types.FieldVal {
			return fs.Obj().(*types.Var).Origin()
		}
		if v, ok := info.Uses[x.Sel].(*types.Var); ok { // pkg.Var
			return v.Origin()
		}
	}
	return nil
}

// lockStmt returns the mutex call made (or deferred) by s, if any.
func lockStmt(info *types.Info, s ast.Stmt) (mutexCall, bool) {
	var call *ast.CallExpr
	switch s := s.(type) {
	case *ast.ExprStmt:
		call, _ = unparen(s.X).(*ast.CallExpr)
	case *ast.DeferStmt:
		call = s.Call
	}
	if call == nil {
		return mutexCall{}, false
	}
	return mutexMethod(info, call)
}

// heldSections returns the Lock calls whose critical section holds the
// mutex across a blocking operation, under the current blocking analysis.
func (p *Program) heldSections() []mutexCall {
	var out []mutexCall
	for _, pkg := range p.Pkgs {
		if isSyncPkg(pkg) {
			continue
		}
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				var list []ast.Stmt
				switch n := n.(type) {
				case *ast.BlockStmt:
					list = n.List
				case *ast.CaseClause:
					list = n.Body
				case *ast.CommClause:
					list = n.Body
				default:
					return true
				}
				for i, s := range list {
					mc, ok := lockStmt(info, s)
					if _, deferred := s.(*ast.DeferStmt); !ok || deferred || !isLock(mc.method) {
						continue
					}
					end, unlocked := len(list), false
					for j := i + 1; j < len(list); j++ {
						u, ok := lockStmt(info, list[j])
						if !ok || !isUnlock(u.method) || u.expr != mc.expr {
							continue
						}
						unlocked = true
						if _, deferred := list[j].(*ast.DeferStmt); !deferred {
							end = j
							break
						}
					}
					if !unlocked {
						out = append(out, mc) // still locked on return
						continue
					}
					for _, t := range list[i+1 : end] {
						if _, ok := p.blocksAt(info, t); ok {
							out = append(out, mc)
							break
						}
					}
				}
				return true
			})
		}
	}
	return out
}

// escapingMutexes returns the direct mutexes whose address escapes (see the
// comment at the top of this file).
func (p *Program) escapingMutexes() map[types.Object]bool {
	esc := map[types.Object]bool{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			callees := map[ast.Expr]bool{}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					callees[unparen(n.Fun)] = true
					if mc, ok := mutexMethod(info, n); ok && mc.method == "RLocker" && mc.key != nil {
						esc[mc.key] = true
					}
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						if k := mutexKey(info, n.X); k != nil {
							esc[k] = true
						}
					}
				case *ast.SelectorExpr:
					if s, ok := info.Selections[n]; ok && s.Kind() == types.MethodVal && !callees[n] {
						if k := mutexKey(info, n.X); k != nil {
							esc[k] = true
						}
					}
				}
				return true
			})
		}
	}
	// Structs embedding a mutex, stored in interface values.
	for _, name := range []string{"Lock", "RLock"} {
		for _, impl := range p.ifaceImpls[name] {
			obj, index, _ := types.LookupFieldOrMethod(impl.typ, false, impl.fn.Pkg(), name)
			if fn, ok := obj.(*types.Func); !ok || fn.Pkg() == nil || fn.Pkg().Path() != "sync" || len(index) < 2 {
				continue
			}
			t := impl.typ
			var f *types.Var
			for _, idx := range index[:len(index)-1] {
				base, _ := derefType(t)
				st, ok := base.Underlying().(*types.Struct)
				if !ok {
					f = nil
					break
				}
				f = st.Field(idx)
				t = f.Type()
			}
			if f != nil && syncMutexType(f.Type()) {
				esc[f.Origin()] = true
			}
		}
	}
	return esc
}

// findWaitLocks marks the Lock calls that may have to wait, and reports
// whether it marked any new ones.
func (p *Program) findWaitLocks() bool {
	if p.escMutexes == nil {
		p.escMutexes = p.escapingMutexes()
	}
	held, indirect := map[types.Object]bool{}, false
	for _, mc := range p.heldSections() {
		if mc.direct() {
			held[mc.key] = true
		} else {
			indirect = true
		}
	}
	heldEsc := false
	for k := range held {
		if p.escMutexes[k] {
			heldEsc = true
		}
	}
	changed := false
	for _, mc := range p.lockCalls {
		if mc.slow == nil || p.waitLocks[mc.call] != nil {
			continue
		}
		var wait bool
		if mc.direct() {
			wait = held[mc.key] || (indirect && p.escMutexes[mc.key])
		} else {
			wait = indirect || heldEsc
		}
		if wait {
			p.waitLocks[mc.call] = mc.slow
			changed = true
		}
	}
	return changed
}

// WaitLock returns the waiting function to call instead of a Lock or RLock
// call, or nil. For a sync.Locker call it is lockerSlow, a method of a
// helper type whose receiver is unused, taking the Locker as its argument.
func (p *Program) WaitLock(call *ast.CallExpr) (fn *types.Func, locker bool) {
	fn = p.waitLocks[call]
	return fn, fn != nil && fn.Name() == "lockerSlow"
}

// blocksAt returns the position of the first operation in n that may block.
// Function literals and go statements run elsewhere and are skipped.
func (p *Program) blocksAt(info *types.Info, n ast.Node) (token.Pos, bool) {
	var at token.Pos
	ast.Inspect(n, func(n ast.Node) bool {
		if at.IsValid() {
			return false
		}
		switch n := n.(type) {
		case *ast.FuncLit, *ast.GoStmt:
			return false
		case *ast.SendStmt:
			at = n.Arrow
		case *ast.UnaryExpr:
			if n.Op == token.ARROW {
				at = n.OpPos
			}
		case *ast.SelectStmt:
			blocking := true
			for _, c := range n.Body.List {
				if c.(*ast.CommClause).Comm == nil {
					blocking = false
				}
			}
			if blocking {
				at = n.Select
				return false
			}
			// With a default case the communications do not block; the
			// case bodies still may.
			for _, c := range n.Body.List {
				for _, s := range c.(*ast.CommClause).Body {
					if pos, ok := p.blocksAt(info, s); ok {
						at = pos
						return false
					}
				}
			}
			return false
		case *ast.RangeStmt:
			switch info.TypeOf(n.X).Underlying().(type) {
			case *types.Chan:
				at = n.For
			case *types.Signature:
				if p.RangeBlocks(info, n) {
					at = n.For
				}
			}
		case *ast.CallExpr:
			if p.CallBlocks(info, n) {
				at = n.Lparen
			}
		}
		return !at.IsValid()
	})
	return at, at.IsValid()
}
