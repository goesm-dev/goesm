package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Blocking that depends on the arguments. The blocking analysis resolves a
// call through an interface or function value by every method or function
// the program could store in it, so a function that calls a method of its
// interface parameter blocks for every caller as soon as one type with a
// blocking method is stored in that interface anywhere: fmt.Fprintf(w,
// ...) calls w.Write, and the program's io.PipeWriter makes it async, and
// with it every Formatter that prints with fmt.Fprintf(state, ...), fmt's
// handling of them, and so fmt.Sprintf. sync.Once.Do(f) blocks when any
// func() value of the program does.
//
// A declared function's tracked parameters are those of an interface or
// function type that its body never assigns or takes the address of; its
// parameter uses are the methods it calls on them (or that it calls them),
// directly in its body or by passing them on as tracked parameters of the
// functions it calls. Besides the usual blocking (async), each such function
// gets an intrinsic one (asyncA) that assumes the uses do not block. A call
// of a function that is async but intrinsically not, whose arguments for
// the used parameters do not block (a *fmt.pp passed as an io.Writer, a
// function that does not block passed as a func()), calls a synchronous
// clone of it, name$sync, instead. The clone is the function lowered with
// those uses, and the calls passing its parameters on, synchronous.

// A callSite is a static call of a declared, non-generic function or
// method.
type callSite struct {
	call *ast.CallExpr
	fn   *types.Func
	info *types.Info
}

// A paramCall is a call through a tracked parameter: of the function value,
// or of the interface method m.
type paramCall struct {
	fn  *types.Func
	idx int
	use paramUse
}

// A paramUse is a call of a tracked parameter (m nil) or of its method m;
// via is the interface type the parameter was asserted to first (bw, ok :=
// w.(io.ByteWriter)), or nil.
type paramUse struct {
	m   *types.Func
	via types.Type
}

func (u paramUse) key() string {
	k := ""
	if u.m != nil {
		k = methodKey(u.m)
	}
	if u.via != nil {
		k += " via " + u.via.String()
	}
	return k
}

// A trackedVar is a tracked parameter, or a variable holding it asserted to
// another interface type.
type trackedVar struct {
	idx int
	via types.Type
}

// staticCallee returns the declared, non-generic function or method (of a
// concrete type) call calls, or nil.
func staticCallee(info *types.Info, call *ast.CallExpr) *types.Func {
	var fn *types.Func
	switch f := unparen(call.Fun).(type) {
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
	if fn == nil || fn.Origin() != fn {
		return nil
	}
	sig := fn.Signature()
	if r := sig.Recv(); r != nil && isIface(r.Type()) {
		return nil
	}
	if sig.TypeParams().Len() > 0 || sig.RecvTypeParams().Len() > 0 {
		return nil
	}
	return fn
}

// trackParams finds the tracked parameters of fn, declared by fd, and the
// calls through them in its body.
func (p *Program) trackParams(info *types.Info, fn *types.Func, fd *ast.FuncDecl) {
	sig := fn.Signature()
	if sig.TypeParams().Len() > 0 || sig.RecvTypeParams().Len() > 0 || p.syncOnly[fn] {
		return
	}
	n := sig.Params().Len()
	if sig.Variadic() {
		n--
	}
	tracked := map[*types.Var]trackedVar{}
	for i := 0; i < n; i++ {
		v := sig.Params().At(i)
		if _, isTP := types.Unalias(v.Type()).(*types.TypeParam); isTP {
			continue
		}
		switch v.Type().Underlying().(type) {
		case *types.Interface, *types.Signature:
			tracked[v] = trackedVar{i, nil}
		}
	}
	if len(tracked) == 0 {
		return
	}
	// Variables holding a tracked parameter asserted to an interface type.
	param := func(e ast.Expr) (trackedVar, bool) {
		if id, ok := unparen(e).(*ast.Ident); ok {
			if v, ok := info.Uses[id].(*types.Var); ok {
				t, ok := tracked[v]
				return t, ok && t.via == nil
			}
		}
		return trackedVar{}, false
	}
	alias := func(def *ast.Ident, t trackedVar, to types.Type) {
		if v, ok := info.Defs[def].(*types.Var); ok && isIface(to) {
			if _, isTP := types.Unalias(to).(*types.TypeParam); !isTP {
				tracked[v] = trackedVar{t.idx, to}
			}
		}
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE || len(n.Rhs) != 1 || len(n.Lhs) > 2 {
				break
			}
			if ta, ok := unparen(n.Rhs[0]).(*ast.TypeAssertExpr); ok && ta.Type != nil {
				if t, ok := param(ta.X); ok {
					if id, ok := n.Lhs[0].(*ast.Ident); ok {
						alias(id, t, info.TypeOf(ta.Type))
					}
				}
			}
		case *ast.TypeSwitchStmt:
			as, ok := n.Assign.(*ast.AssignStmt)
			if !ok {
				break
			}
			t, ok := param(as.Rhs[0].(*ast.TypeAssertExpr).X)
			if !ok {
				break
			}
			for _, c := range n.Body.List {
				cc := c.(*ast.CaseClause)
				if v, ok := info.Implicits[cc].(*types.Var); ok && len(cc.List) == 1 && isIface(v.Type()) {
					if _, isTP := types.Unalias(v.Type()).(*types.TypeParam); !isTP {
						tracked[v] = trackedVar{t.idx, v.Type()}
					}
				}
			}
		}
		return true
	})
	drop := func(e ast.Expr) {
		if id, ok := unparen(e).(*ast.Ident); ok {
			if v, ok := info.Uses[id].(*types.Var); ok {
				delete(tracked, v)
			}
		}
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt: // := records a variable it redeclares as a use
			for _, l := range n.Lhs {
				drop(l)
			}
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				drop(n.Key)
				drop(n.Value)
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				drop(n.X)
			}
		}
		return true
	})
	if len(tracked) == 0 {
		return
	}
	p.tracked[fn] = tracked
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false // its own unit
		case *ast.CallExpr:
			switch f := unparen(n.Fun).(type) {
			case *ast.Ident:
				if v, ok := info.Uses[f].(*types.Var); ok {
					if t, ok := tracked[v]; ok {
						if _, isFunc := v.Type().Underlying().(*types.Signature); isFunc {
							p.paramCalls[n] = paramCall{fn, t.idx, paramUse{}}
						}
					}
				}
			case *ast.SelectorExpr:
				x, ok := unparen(f.X).(*ast.Ident)
				sel := info.Selections[f]
				if !ok || sel == nil || sel.Kind() != types.MethodVal {
					break
				}
				if v, ok := info.Uses[x].(*types.Var); ok {
					if t, ok := tracked[v]; ok && isIface(v.Type()) {
						p.paramCalls[n] = paramCall{fn, t.idx, paramUse{sel.Obj().(*types.Func), t.via}}
					}
				}
			}
		}
		return true
	})
}

// findParamUses computes the parameter uses of every function with tracked
// parameters: the calls through them, and the uses of the parameters of
// the functions they are passed on to.
func (p *Program) findParamUses() {
	add := func(fn *types.Func, i int, u paramUse) bool {
		us := p.paramUses[fn]
		if us == nil {
			us = make([]map[string]paramUse, fn.Signature().Params().Len())
			p.paramUses[fn] = us
		}
		if us[i] == nil {
			us[i] = map[string]paramUse{}
		}
		k := u.key()
		if _, ok := us[i][k]; ok {
			return false
		}
		us[i][k] = u
		return true
	}
	for _, c := range p.paramCalls {
		add(c.fn, c.idx, c.use)
	}
	for changed := true; changed; {
		changed = false
		for _, u := range p.units {
			fn, ok := u.key.(*types.Func)
			tracked := p.tracked[fn]
			if !ok || tracked == nil {
				continue
			}
			for _, s := range u.sites {
				for k, us := range p.paramUses[s.fn] {
					if len(us) == 0 {
						continue
					}
					arg := siteArg(s, k)
					id, ok := unparen(arg).(*ast.Ident)
					if !ok {
						continue
					}
					v, ok := s.info.Uses[id].(*types.Var)
					if !ok {
						continue
					}
					if t, ok := tracked[v]; ok {
						for _, use := range us {
							if use.via == nil {
								// The callee's own assertion is the
								// stricter one; keeping t's is sound.
								use.via = t.via
							}
							if add(fn, t.idx, use) {
								changed = true
							}
						}
					}
				}
			}
		}
	}
}

// siteArg returns the argument of call site s for parameter k, or nil
// (f(g()) passes a tuple).
func siteArg(s callSite, k int) ast.Expr {
	sig := s.fn.Signature()
	if len(s.call.Args) == 1 && sig.Params().Len() > 1 {
		if _, ok := s.info.TypeOf(s.call.Args[0]).(*types.Tuple); ok {
			return nil
		}
	}
	if k >= len(s.call.Args) {
		return nil
	}
	return s.call.Args[k]
}

// siteBlocks reports whether call site s blocks. In the clone of assume,
// assume's tracked parameters do not block when passed on. Once the clones
// are settled (cloned is set), a call of a function that blocks is
// synchronous only when the function has a clone.
func (p *Program) siteBlocks(s callSite, assume *types.Func) bool {
	c := s.fn
	if !p.async[c] || p.syncOnly[c] {
		return false
	}
	uses := p.paramUses[c]
	if uses == nil || p.asyncA[c] || p.cloned != nil && !p.cloned[c] {
		return true
	}
	for k, us := range uses {
		if len(us) == 0 {
			continue
		}
		arg := siteArg(s, k)
		if arg == nil || p.argBlocks(s.info, arg, us, assume) {
			return true
		}
	}
	return false
}

// SyncClone reports whether call calls the synchronous clone of its
// function (see above). assume is the function whose clone the call is
// in, or nil.
func (p *Program) SyncClone(info *types.Info, call *ast.CallExpr, assume *types.Func) bool {
	if p.paramCalls[call].fn != nil {
		return false
	}
	fn := staticCallee(info, call)
	if fn == nil || !p.cloned[fn] {
		return false
	}
	return !p.siteBlocks(callSite{call, fn, info}, assume)
}

// HasClone reports whether fn has a synchronous clone.
func (p *Program) HasClone(fn *types.Func) bool { return p.cloned[fn] }

// argBlocks reports whether passing arg for a parameter with the uses us
// may block.
func (p *Program) argBlocks(info *types.Info, arg ast.Expr, us map[string]paramUse, assume *types.Func) bool {
	e := unparen(arg)
	if id, ok := e.(*ast.Ident); ok {
		switch obj := info.Uses[id].(type) {
		case *types.Nil:
			return false
		case *types.Var:
			if _, ok := p.tracked[assume][obj]; ok && assume != nil {
				return false // passed on: its uses are assume's
			}
		}
	}
	t := info.TypeOf(e)
	if _, isTP := types.Unalias(t).(*types.TypeParam); isTP {
		return true
	}
	for _, u := range us {
		if u.m == nil {
			if p.funcArgBlocks(info, e) {
				return true
			}
			continue
		}
		obj, _, _ := types.LookupFieldOrMethod(t, true, u.m.Pkg(), u.m.Name())
		fn, ok := obj.(*types.Func)
		switch {
		case !ok && u.via == nil:
			return true
		case !ok && !isIface(t):
			// A concrete type without the method: the assertion to via
			// fails.
		case !ok:
			// The dynamic types that have both t's methods and via's.
			both := types.NewInterfaceType(nil, []types.Type{t, u.via}).Complete()
			if p.ifaceCallBlocks(both, u.m, map[types.Type]bool{}) {
				return true
			}
		case fn.Signature().Recv() != nil && isIface(fn.Signature().Recv().Type()):
			r := fn.Signature().Recv().Type()
			if isIface(t) {
				r = t
			}
			if p.ifaceCallBlocks(r, fn, map[types.Type]bool{}) {
				return true
			}
		case p.async[fn.Origin()]:
			return true
		}
	}
	return false
}

// funcArgBlocks reports whether calling the function value e may block.
func (p *Program) funcArgBlocks(info *types.Info, e ast.Expr) bool {
	switch f := e.(type) {
	case *ast.FuncLit:
		return p.async[f]
	case *ast.Ident:
		switch obj := info.Uses[f].(type) {
		case *types.Func:
			if obj.Origin() == obj {
				return p.async[obj]
			}
		case *types.Var:
			if lit := p.localLits[obj]; lit != nil {
				return p.async[lit]
			}
		}
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[f]; ok {
			fn, _ := sel.Obj().(*types.Func)
			if sel.Kind() != types.MethodVal || fn == nil || p.waitLockVals[f] != nil {
				return true
			}
			if recv := fn.Signature().Recv(); recv != nil && isIface(recv.Type()) {
				return p.ifaceCallBlocks(sel.Recv(), fn, map[types.Type]bool{})
			}
			return p.async[fn.Origin()]
		}
		if fn, ok := info.Uses[f.Sel].(*types.Func); ok && fn.Origin() == fn {
			return p.async[fn]
		}
	}
	return true
}

// findClones settles which functions get a synchronous clone: those some
// call (in a function or literal, or in another clone) can use.
func (p *Program) findClones() {
	cloned := map[*types.Func]bool{}
	byFunc := map[*types.Func]*unit{}
	var work []*types.Func
	visit := func(u *unit, assume *types.Func) {
		for _, s := range u.sites {
			c := s.fn
			if !cloned[c] && p.async[c] && p.paramUses[c] != nil && !p.asyncA[c] && byFunc[c] != nil && !p.siteBlocks(s, assume) {
				cloned[c] = true
				work = append(work, c)
			}
		}
	}
	for _, u := range p.units {
		if fn, ok := u.key.(*types.Func); ok {
			byFunc[fn] = u
		}
	}
	for _, u := range p.units {
		visit(u, nil)
	}
	for len(work) > 0 {
		fn := work[len(work)-1]
		work = work[:len(work)-1]
		visit(byFunc[fn], fn)
	}
	p.cloned = cloned
}

// Function parameters that do not escape. Calls of function values are
// resolved by signature over all the function values of the program, so
// every func() value that blocks makes sync.Once.Do, and every function
// that takes a func() and calls it, block for all callers. A tracked
// function parameter that its function only calls, compares with nil or
// passes on as such a parameter (at a static call) does not escape: its
// values are the arguments of the static calls of the function. Those
// arguments, when function literals, functions or method values, are then
// not function values, and the calls through the parameter block only when
// one of them may (or, when the function itself can be called dynamically,
// when a function value may).

// A paramDyn is a call through the function parameter idx of fn.
type paramDyn struct {
	fn  *types.Func
	idx int
	sig *types.Signature
}

type paramKey struct {
	fn  *types.Func
	idx int
}

// A siteRef is a static call site and the declared function whose body it
// is directly in, or nil.
type siteRef struct {
	s     callSite
	owner *types.Func
}

// findNonEscaping finds the function parameters that do not escape (see
// above) and the arguments passed only as them.
func (p *Program) findNonEscaping() {
	p.nonEsc = map[*types.Func]map[int]bool{}
	p.argOnly = map[ast.Expr]bool{}
	p.sitesOf = map[*types.Func][]siteRef{}
	p.unsited = map[*types.Func]bool{}
	sited := map[*ast.CallExpr]bool{}
	for _, u := range p.units {
		owner, _ := u.key.(*types.Func)
		for _, s := range u.sites {
			p.sitesOf[s.fn] = append(p.sitesOf[s.fn], siteRef{s, owner})
			sited[s.call] = true
		}
	}
	// The parameters each one is passed on as.
	fwd := map[paramKey][]paramKey{}
	esc := map[paramKey]bool{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && !sited[call] {
					if fn := staticCallee(info, call); fn != nil {
						p.unsited[fn] = true
					}
				}
				return true
			})
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				fn, _ := info.Defs[fd.Name].(*types.Func)
				params := map[*types.Var]int{}
				for v, t := range p.tracked[fn] {
					if _, ok := v.Type().Underlying().(*types.Signature); ok && t.via == nil {
						params[v] = t.idx
						esc[paramKey{fn, t.idx}] = false
					}
				}
				if len(params) == 0 {
					continue
				}
				var stack []ast.Node
				lits := 0
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if n == nil {
						if _, ok := stack[len(stack)-1].(*ast.FuncLit); ok {
							lits--
						}
						stack = stack[:len(stack)-1]
						return true
					}
					var parent ast.Node
					if len(stack) > 0 {
						parent = stack[len(stack)-1]
					}
					stack = append(stack, n)
					switch n := n.(type) {
					case *ast.FuncLit:
						lits++
					case *ast.Ident:
						v, _ := info.Uses[n].(*types.Var)
						idx, ok := params[v]
						if !ok {
							break
						}
						k := paramKey{fn, idx}
						if lits > 0 || parent == nil {
							esc[k] = true
							break
						}
						switch par := parent.(type) {
						case *ast.CallExpr:
							if par.Fun == n {
								break
							}
							j := argIndex(par, n)
							if j < 0 || !sited[par] {
								esc[k] = true
								break
							}
							callee := staticCallee(info, par)
							if p.trackedFuncParam(callee, j) {
								fwd[k] = append(fwd[k], paramKey{callee, j})
							} else {
								esc[k] = true
							}
						case *ast.BinaryExpr:
							other := par.X
							if other == n {
								other = par.Y
							}
							if _, isNil := info.Uses[identOf(other)].(*types.Nil); !isNil || par.Op != token.EQL && par.Op != token.NEQ {
								esc[k] = true
							}
						default:
							esc[k] = true
						}
					}
					return true
				})
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for k, to := range fwd {
			if esc[k] {
				continue
			}
			for _, t := range to {
				if e, ok := esc[t]; !ok || e {
					esc[k] = true
					changed = true
					break
				}
			}
		}
	}
	for k, e := range esc {
		if !e {
			if p.nonEsc[k.fn] == nil {
				p.nonEsc[k.fn] = map[int]bool{}
			}
			p.nonEsc[k.fn][k.idx] = true
		}
	}
	p.findLocalLits(sited)
	for fn, sites := range p.sitesOf {
		for idx := range p.nonEsc[fn] {
			for _, r := range sites {
				arg := siteArg(r.s, idx)
				if arg == nil {
					continue
				}
				switch e := unparen(arg).(type) {
				case *ast.FuncLit:
					p.argOnly[e] = true
				case *ast.Ident:
					if _, ok := r.s.info.Uses[e].(*types.Func); ok {
						p.argOnly[e] = true
					}
				case *ast.SelectorExpr:
					if sel, ok := r.s.info.Selections[e]; ok {
						if sel.Kind() == types.MethodVal {
							p.argOnly[e] = true
						}
					} else if _, ok := r.s.info.Uses[e.Sel].(*types.Func); ok {
						p.argOnly[e] = true
					}
				}
			}
		}
	}
}

// findLocalLits finds the local variables that hold a function literal
// (g := func() {...}) and are only passed as non-escaping parameters, like
// the g that sync.OnceFunc passes to Once.Do: the literal is then not a
// function value either.
func (p *Program) findLocalLits(sited map[*ast.CallExpr]bool) {
	p.localLits = map[*types.Var]*ast.FuncLit{}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				lits := map[*types.Var]*ast.FuncLit{}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					as, ok := n.(*ast.AssignStmt)
					if !ok || as.Tok != token.DEFINE || len(as.Lhs) != len(as.Rhs) {
						return true
					}
					for i, l := range as.Lhs {
						lit, ok := unparen(as.Rhs[i]).(*ast.FuncLit)
						if id, isID := l.(*ast.Ident); ok && isID {
							if v, ok := info.Defs[id].(*types.Var); ok {
								lits[v] = lit
							}
						}
					}
					return true
				})
				if len(lits) == 0 {
					continue
				}
				var stack []ast.Node
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if n == nil {
						stack = stack[:len(stack)-1]
						return true
					}
					var parent ast.Node
					if len(stack) > 0 {
						parent = stack[len(stack)-1]
					}
					stack = append(stack, n)
					id, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					v, _ := info.Uses[id].(*types.Var)
					if lits[v] == nil {
						return true
					}
					call, ok := parent.(*ast.CallExpr)
					j := -1
					if ok && sited[call] {
						j = argIndex(call, id)
					}
					if j < 0 || !p.nonEsc[staticCallee(info, call)][j] {
						delete(lits, v)
					}
					return true
				})
				for v, lit := range lits {
					p.localLits[v] = lit
					p.argOnly[lit] = true
				}
			}
		}
	}
}

// argIndex returns the index of arg among call's arguments, or -1.
func argIndex(call *ast.CallExpr, arg ast.Expr) int {
	for i, a := range call.Args {
		if a == arg {
			return i
		}
	}
	return -1
}

// trackedFuncParam reports whether parameter j of fn is a tracked function
// parameter.
func (p *Program) trackedFuncParam(fn *types.Func, j int) bool {
	if fn == nil || j >= fn.Signature().Params().Len() {
		return false
	}
	v := fn.Signature().Params().At(j)
	t, ok := p.tracked[fn][v]
	_, isFunc := v.Type().Underlying().(*types.Signature)
	return ok && t.via == nil && isFunc
}

// dynCallable reports whether fn may be called other than at its static
// call sites: through a function value, a method expression or an
// interface, from a go statement or package initializer, or through a
// //go:linkname pull. Methods of the types stored in interface values are
// called through interfaces only when an interface has them.
func (p *Program) dynCallable(fn *types.Func) bool {
	if p.funcValues[fn] || p.unsited[fn] {
		return true
	}
	for _, t := range p.linkTargets {
		if t == fn {
			return true
		}
	}
	if fn.Signature().Recv() != nil {
		if len(p.methodExprs[fn.Name()]) > 0 {
			return true
		}
		for _, im := range p.ifaceImpls[fn.Name()] {
			if im.fn.Origin() == fn && p.ifaceHas(fn) {
				return true
			}
		}
	}
	return false
}

// ifaceHas reports whether an interface type of the program has the
// method fn (see findDynMethods).
func (p *Program) ifaceHas(fn *types.Func) bool {
	own := stripRecv(fn.Signature())
	for _, m := range p.ifaceMethods[fn.Name()] {
		if m.loose || types.Identical(m.sig, own) {
			return true
		}
	}
	return false
}

// paramBlocks reports whether a call through the non-escaping function
// parameter idx of fn, of signature sig, may block: whether an argument of
// a static call of fn may, or, when fn is callable dynamically, a function
// value may.
func (p *Program) paramBlocks(fn *types.Func, idx int, sig *types.Signature, seen map[paramKey]bool) bool {
	k := paramKey{fn, idx}
	if seen[k] {
		return false
	}
	seen[k] = true
	if p.dynCallable(fn) && p.dynSigBlocks(sig, &unit{encl: fn}, p.units) {
		return true
	}
	for _, r := range p.sitesOf[fn] {
		arg := siteArg(r.s, idx)
		if arg == nil {
			return true
		}
		switch e := unparen(arg).(type) {
		case *ast.Ident:
			switch obj := r.s.info.Uses[e].(type) {
			case *types.Nil:
				continue
			case *types.Func:
				if p.funcArgBlocks(r.s.info, e) {
					return true
				}
				continue
			case *types.Var:
				if p.localLits[obj] != nil {
					if p.funcArgBlocks(r.s.info, e) {
						return true
					}
					continue
				}
				if t, ok := p.tracked[r.owner][obj]; ok && r.owner != nil && p.nonEsc[r.owner][t.idx] && t.via == nil {
					if p.paramBlocks(r.owner, t.idx, sig, seen) {
						return true
					}
					continue
				}
			}
		case *ast.FuncLit, *ast.SelectorExpr:
			if p.argOnly[e] {
				if p.funcArgBlocks(r.s.info, e) {
					return true
				}
				continue
			}
		}
		if p.dynSigBlocks(sig, &unit{encl: r.owner}, p.units) {
			return true
		}
	}
	return false
}
