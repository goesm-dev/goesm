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
// blocked. Such critical sections are found here. A Lock or RLock call
// statement opens a section that lasts until an Unlock or RUnlock of the
// same expression later in the same statement list, or to the end of the
// list after a deferred unlock. If the section may block, the mutex (the
// variable or struct field locked) is held across a blocking operation, and
// every Lock and RLock of that variable or field in the program is lowered
// to the waiting lockSlow or rLockSlow. Those calls are async, which can make
// more sections block, so this repeats with the blocking analysis until
// nothing changes.
//
// A mutex reached through a pointer variable (mu *sync.Mutex) may also be
// locked under other names, which do not wait; such sections in code outside
// the standard library get a warning (Program.Notes), as do sections whose
// mutex has no name at all (m[k].mu.Lock()).

// mutexCall is a call of a sync.Mutex or sync.RWMutex method.
type mutexCall struct {
	call   *ast.CallExpr
	method string       // Lock, RLock, Unlock, ...
	key    types.Object // the variable or field locked; nil if unnamed
	ptr    bool         // key is a pointer to the mutex
	expr   string       // the mutex expression, for messages
	slow   *types.Func  // the waiting variant of Lock and RLock
}

// mutexMethod recognizes a call of a sync.Mutex or sync.RWMutex method.
func mutexMethod(info *types.Info, call *ast.CallExpr) (mutexCall, bool) {
	sel, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return mutexCall{}, false
	}
	s, ok := info.Selections[sel]
	if !ok || s.Kind() != types.MethodVal {
		return mutexCall{}, false
	}
	fn := s.Obj().(*types.Func)
	if fn.Pkg() == nil || fn.Pkg().Path() != "sync" {
		return mutexCall{}, false
	}
	ptr, ok := fn.Signature().Recv().Type().(*types.Pointer)
	if !ok {
		return mutexCall{}, false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || (named.Obj().Name() != "Mutex" && named.Obj().Name() != "RWMutex") {
		return mutexCall{}, false
	}
	mc := mutexCall{call: call, method: fn.Name(), expr: types.ExprString(sel.X)}
	if slow := map[string]string{"Lock": "lockSlow", "RLock": "rLockSlow"}[fn.Name()]; slow != "" {
		obj, _, _ := types.LookupFieldOrMethod(ptr, false, fn.Pkg(), slow)
		mc.slow, _ = obj.(*types.Func)
	}
	// The mutex: the last embedded field on the path, or the operand.
	if path := s.Index(); len(path) > 1 {
		t := info.TypeOf(sel.X)
		var f *types.Var
		for _, idx := range path[:len(path)-1] {
			base, _ := derefType(t)
			f = base.Underlying().(*types.Struct).Field(idx)
			t = f.Type()
		}
		mc.key = f.Origin()
		_, mc.ptr = f.Type().Underlying().(*types.Pointer)
		return mc, true
	}
	x := unparen(sel.X)
	if star, ok := x.(*ast.StarExpr); ok {
		x = unparen(star.X)
	}
	switch x := x.(type) {
	case *ast.Ident:
		if v, ok := info.ObjectOf(x).(*types.Var); ok {
			mc.key = v.Origin()
		}
	case *ast.SelectorExpr:
		if fs, ok := info.Selections[x]; ok && fs.Kind() == types.FieldVal {
			mc.key = fs.Obj().(*types.Var).Origin()
		} else if v, ok := info.Uses[x.Sel].(*types.Var); ok { // pkg.Var
			mc.key = v.Origin()
		}
	}
	if mc.key != nil {
		_, mc.ptr = mc.key.Type().Underlying().(*types.Pointer)
	}
	return mc, true
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

// lockSection is a critical section that may block.
type lockSection struct {
	std bool // in the standard library
	mc  mutexCall
	at  token.Pos // the blocking operation
}

// blockingSections returns the critical sections in the program that may
// block under the current blocking analysis.
func (p *Program) blockingSections() []lockSection {
	var out []lockSection
	for _, pkg := range p.Pkgs {
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
					if _, deferred := s.(*ast.DeferStmt); !ok || deferred || (mc.method != "Lock" && mc.method != "RLock") {
						continue
					}
					end := len(list)
					for j := i + 1; j < len(list); j++ {
						if _, deferred := list[j].(*ast.DeferStmt); deferred {
							continue // unlocks at return: the section lasts to the end
						}
						if u, ok := lockStmt(info, list[j]); ok && (u.method == "Unlock" || u.method == "RUnlock") && u.expr == mc.expr {
							end = j
							break
						}
					}
					for _, t := range list[i+1 : end] {
						if pos, ok := p.blocksAt(info, t); ok {
							out = append(out, lockSection{p.std[pkg], mc, pos})
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

// findWaitLocks marks the Lock and RLock calls of mutexes held across a
// blocking operation as waiting calls, and reports whether it marked any new
// ones.
func (p *Program) findWaitLocks() bool {
	held := map[types.Object]bool{}
	for _, s := range p.blockingSections() {
		if s.mc.key != nil {
			held[s.mc.key] = true
		}
	}
	changed := false
	for _, mc := range p.lockCalls {
		if mc.slow != nil && held[mc.key] && p.waitLocks[mc.call] == nil {
			p.waitLocks[mc.call] = mc.slow
			changed = true
		}
	}
	return changed
}

// noteLocks warns about the critical sections whose mutex may be locked
// under other names (see the comment at the top of this file).
func (p *Program) noteLocks() {
	for _, s := range p.blockingSections() {
		if s.std {
			continue
		}
		switch {
		case s.mc.key == nil:
			p.Notes = append(p.Notes, Diagnostic{Pos: p.Fset.Position(s.at),
				Msg: s.mc.expr + " is locked across an operation that may block; goesm's locks only wait for a mutex in a variable or struct field, so if another goroutine locks it meanwhile, the program panics"})
		case s.mc.ptr:
			p.Notes = append(p.Notes, Diagnostic{Pos: p.Fset.Position(s.at),
				Msg: s.mc.expr + " is locked across an operation that may block; goesm's locks wait only where the mutex is locked as " + s.mc.expr + ", so if another goroutine locks it under another name meanwhile, the program panics"})
		}
	}
}

// WaitLock returns the waiting method to call instead of a sync.Mutex or
// sync.RWMutex Lock or RLock call, or nil.
func (p *Program) WaitLock(call *ast.CallExpr) *types.Func { return p.waitLocks[call] }

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
