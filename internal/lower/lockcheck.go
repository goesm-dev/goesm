package lower

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
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
	if !ok {
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
	mc, ok := mutexSel(info, sel, x)
	mc.call = call
	return mc, ok
}

// mutexSel recognizes a sync.Mutex, sync.RWMutex or sync.Locker method
// selected by sel, with receiver x (nil for a method expression value).
func mutexSel(info *types.Info, sel *ast.SelectorExpr, x ast.Expr) (mutexCall, bool) {
	s, ok := info.Selections[sel]
	if !ok || (s.Kind() != types.MethodVal && s.Kind() != types.MethodExpr) {
		return mutexCall{}, false
	}
	fn := s.Obj().(*types.Func)
	if fn.Pkg() == nil || fn.Pkg().Path() != "sync" {
		return mutexCall{}, false
	}
	mc := mutexCall{method: fn.Name()}
	if x != nil {
		mc.expr = types.ExprString(x)
	} else {
		mc.expr = types.ExprString(sel)
	}
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
	if x == nil {
		return mc, true // a method expression value: the mutex has no name
	}
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
			inList := map[*ast.CallExpr]bool{} // Lock statements of statement lists
			callees := map[ast.Expr]bool{}
			var stack []ast.Node // the enclosing nodes of n
			ast.Inspect(f, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return true
				}
				stack = append(stack, n)
				var list []ast.Stmt
				switch n := n.(type) {
				case *ast.CallExpr:
					callees[unparen(n.Fun)] = true
					// A section that statements do not delimit counts as
					// held: one started by a TryLock that succeeds, or by a
					// Lock outside a statement list (in an if, for or
					// switch initializer, an argument, a deferred Lock).
					if mc, ok := mutexMethod(info, n); ok {
						if mc.method == "TryLock" || mc.method == "TryRLock" || (isLock(mc.method) && !inList[n]) {
							out = append(out, mc)
						}
					}
					return true
				case *ast.SelectorExpr:
					// A Lock method value or expression called later: an
					// indirect section nothing delimits.
					if sel, ok := info.Selections[n]; ok && !callees[n] && (sel.Kind() == types.MethodVal || sel.Kind() == types.MethodExpr) {
						if fn := sel.Obj().(*types.Func); fn.Pkg() != nil && fn.Pkg().Path() == "sync" && (isLock(fn.Name()) || fn.Name() == "TryLock" || fn.Name() == "TryRLock") {
							out = append(out, mutexCall{method: fn.Name(), expr: types.ExprString(n)})
						}
					}
					return true
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
					inList[mc.call] = true
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
					section := list[i+1 : end]
					if !unlocked {
						// Locked in an if (or block) and unlocked by a
						// later statement of an enclosing list, as in
						// if c { mu.Lock() }; ...; if c { mu.Unlock() }.
						if rest, ok := p.unlockedLater(info, stack, mc.expr); ok {
							section = append(append([]ast.Stmt(nil), section...), rest...)
							unlocked = true
						}
					}
					if !unlocked || p.bypasses(info, section, mc.expr, false, false) {
						out = append(out, mc) // still locked on return
						continue
					}
					for _, t := range section {
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

// bypasses reports whether a path through list leaves it (return, panic,
// runtime.Goexit or a call of a function that may call it, or a break,
// continue or goto out of it) before an Unlock of expr: the function may
// then return with the mutex locked. inLoop and
// inBreakable tell whether an unlabeled continue or break stays inside list.
func (p *Program) bypasses(info *types.Info, list []ast.Stmt, expr string, inLoop, inBreakable bool) bool {
	for _, s := range list {
		if u, ok := lockStmt(info, s); ok && isUnlock(u.method) && u.expr == expr {
			return false // the rest of this path is unlocked (or unlocks on return)
		}
		switch s := s.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.ExprStmt, *ast.AssignStmt, *ast.DeclStmt:
			if c, ok := s.(*ast.ExprStmt); ok {
				if c, ok := unparen(c.X).(*ast.CallExpr); ok {
					if b, ok := info.Uses[identOf(unparen(c.Fun))].(*types.Builtin); ok && b.Name() == "panic" {
						return true
					}
				}
			}
			if p.callsGoexit(info, s) {
				return true
			}
		case *ast.BranchStmt:
			switch {
			case s.Tok == token.FALLTHROUGH:
			case s.Label != nil || s.Tok == token.GOTO:
				return true
			case s.Tok == token.CONTINUE && !inLoop, s.Tok == token.BREAK && !inBreakable:
				return true
			}
		case *ast.LabeledStmt:
			if p.bypasses(info, []ast.Stmt{s.Stmt}, expr, inLoop, inBreakable) {
				return true
			}
		case *ast.BlockStmt:
			if p.bypasses(info, s.List, expr, inLoop, inBreakable) {
				return true
			}
		case *ast.IfStmt:
			if (s.Init != nil && p.callsGoexit(info, s.Init)) || p.callsGoexit(info, s.Cond) {
				return true
			}
			if p.bypasses(info, s.Body.List, expr, inLoop, inBreakable) ||
				(s.Else != nil && p.bypasses(info, []ast.Stmt{s.Else}, expr, inLoop, inBreakable)) {
				return true
			}
		case *ast.ForStmt:
			if p.bypasses(info, s.Body.List, expr, true, true) {
				return true
			}
		case *ast.RangeStmt:
			if p.bypasses(info, s.Body.List, expr, true, true) {
				return true
			}
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			var body *ast.BlockStmt
			switch s := s.(type) {
			case *ast.SwitchStmt:
				body = s.Body
			case *ast.TypeSwitchStmt:
				body = s.Body
			case *ast.SelectStmt:
				body = s.Body
			}
			for _, c := range body.List {
				var stmts []ast.Stmt
				switch c := c.(type) {
				case *ast.CaseClause:
					stmts = c.Body
				case *ast.CommClause:
					stmts = c.Body
				}
				if p.bypasses(info, stmts, expr, inLoop, true) {
					return true
				}
			}
		}
	}
	return false
}

// callsGoexit reports whether n calls runtime.Goexit, or a function that
// may call it (see goexits), outside function literals and go statements.
func (p *Program) callsGoexit(info *types.Info, n ast.Node) bool {
	if p.goexits == nil {
		p.goexits = p.findGoexits()
	}
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit, *ast.GoStmt:
			return false
		case *ast.CallExpr:
			if fn, ok := info.Uses[identOf(unparen(n.Fun))].(*types.Func); ok && (isGoexit(fn) || p.goexits[fn.Origin()]) {
				found = true
			}
		}
		return !found
	})
	return found
}

func isGoexit(fn *types.Func) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == "runtime" && fn.Name() == "Goexit"
}

// findGoexits returns the functions and literals that may call
// runtime.Goexit directly or through the functions they call statically.
func (p *Program) findGoexits() map[any]bool {
	exits := map[any]bool{}
	for changed := true; changed; {
		changed = false
		for _, u := range p.units {
			if exits[u.key] {
				continue
			}
			calls := u.goexit
			for _, c := range u.callees {
				calls = calls || exits[c]
			}
			for _, s := range u.sites {
				calls = calls || exits[s.fn]
			}
			if calls {
				exits[u.key] = true
				changed = true
			}
		}
	}
	return exits
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
	wait := func(mc mutexCall) bool {
		if mc.direct() {
			return held[mc.key] || (indirect && p.escMutexes[mc.key])
		}
		return indirect || heldEsc
	}
	changed := false
	for _, mc := range p.lockCalls {
		if mc.slow != nil && p.waitLocks[mc.call] == nil && wait(mc) {
			p.waitLocks[mc.call] = mc.slow
			changed = true
		}
	}
	for _, lv := range p.lockVals {
		if p.waitLockVals[lv.sel] == nil && wait(lv.mc) {
			p.waitLockVals[lv.sel] = lv.mc.slow
			changed = true
		}
	}
	return changed
}

// lockVal is a Lock or RLock method value (mu.Lock) or method expression
// value ((*sync.Mutex).Lock). If its Lock may have to wait, it is bound to
// the waiting variant and the function value is async: dynamic calls of its
// signature may then block.
type lockVal struct {
	sel *ast.SelectorExpr
	mc  mutexCall
	sig *types.Signature
}

// addLockVal records sel, a method value with receiver x or (x nil) a
// method expression used as a value, if it is a mutex's Lock or RLock.
func (p *Program) addLockVal(pkg *packages.Package, sel *ast.SelectorExpr, x ast.Expr) {
	if isSyncPkg(pkg) {
		return
	}
	mc, ok := mutexSel(pkg.TypesInfo, sel, x)
	if !ok || mc.slow == nil || !isLock(mc.method) {
		return
	}
	sig, _ := pkg.TypesInfo.TypeOf(sel).Underlying().(*types.Signature)
	if sig != nil {
		p.lockVals = append(p.lockVals, lockVal{sel, mc, sig})
	}
}

// WaitLockVal returns the waiting function a Lock method value or method
// expression value is bound to, or nil (see WaitLock).
func (p *Program) WaitLockVal(sel *ast.SelectorExpr) (fn *types.Func, locker bool) {
	fn = p.waitLockVals[sel]
	return fn, fn != nil && fn.Name() == "lockerSlow"
}

// WaitLock returns the waiting function to call instead of a Lock or RLock
// call, or nil. For a sync.Locker call it is lockerSlow, a method of a
// helper type whose receiver is unused, taking the Locker as its argument.
func (p *Program) WaitLock(call *ast.CallExpr) (fn *types.Func, locker bool) {
	fn = p.waitLocks[call]
	return fn, fn != nil && fn.Name() == "lockerSlow"
}

// blocksAt returns the position of the first operation in n that may block.
// Function literals and the calls of go statements run elsewhere and are
// skipped; a go statement's function value and arguments are evaluated here.
func (p *Program) blocksAt(info *types.Info, n ast.Node) (token.Pos, bool) {
	var at token.Pos
	ast.Inspect(n, func(n ast.Node) bool {
		if at.IsValid() {
			return false
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.GoStmt:
			for _, e := range append([]ast.Expr{n.Call.Fun}, n.Call.Args...) {
				if pos, ok := p.blocksAt(info, e); ok {
					at = pos
					break
				}
			}
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

// unlockedLater finds, for a Lock statement in the statement list of
// stack's last node that the list does not unlock, a later statement of an
// enclosing statement list that unlocks the same expression: the list must
// be a block or an if or else branch (not a loop body or case), directly or
// through other such statements. It returns the statements of the
// enclosing lists that run between, up to and including that statement.
func (p *Program) unlockedLater(info *types.Info, stack []ast.Node, expr string) ([]ast.Stmt, bool) {
	var between []ast.Stmt
	k := len(stack) - 1 // the node owning the list
	if _, ok := stack[k].(*ast.BlockStmt); !ok {
		return nil, false
	}
	for k > 0 {
		// Climb from the block through if statements (else if chains) and
		// plain blocks to the statement in an enclosing list.
		child := stack[k]
		parent := stack[k-1]
		switch parent := parent.(type) {
		case *ast.IfStmt:
			if child != ast.Node(parent.Body) && child != parent.Else {
				return nil, false // in the condition
			}
			k--
			continue
		case *ast.BlockStmt:
		case *ast.CaseClause, *ast.CommClause:
		default:
			return nil, false
		}
		var list []ast.Stmt
		switch parent := parent.(type) {
		case *ast.BlockStmt:
			list = parent.List
		case *ast.CaseClause:
			list = parent.Body
		case *ast.CommClause:
			list = parent.Body
		}
		idx := -1
		for i, st := range list {
			if ast.Node(st) == child {
				idx = i
			}
		}
		if idx < 0 {
			return nil, false
		}
		for _, st := range list[idx+1:] {
			between = append(between, st)
			if containsUnlock(info, st, expr) {
				return between, true
			}
		}
		if _, ok := parent.(*ast.BlockStmt); !ok {
			return nil, false
		}
		k--
	}
	return nil, false
}

// containsUnlock reports whether s unlocks expr (outside function literals).
func containsUnlock(info *types.Info, s ast.Stmt, expr string) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if mc, ok := mutexMethod(info, n); ok && isUnlock(mc.method) && mc.expr == expr {
				found = true
			}
		}
		return !found
	})
	return found
}
