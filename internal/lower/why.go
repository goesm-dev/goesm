package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

// Why a function blocks (goesm emit-ts -blocking).
//
// A function that may block is lowered as an async function, and returns a
// Promise to JavaScript. A tool that cannot take a Promise somewhere (a
// framework that calls a function synchronously) has to tell its user why
// the function blocks, down to the operation that waits: through the calls
// of declared functions, but also through the calls of interface methods
// and of function values, whose callees only the blocking analysis knows.
// The explanation reads the analysis's results (async, waitLocks, ...) once
// they are final; it never changes them. Each step is the first operation
// or call, in source order, that makes the function block, the way the
// lowering decides to await it, and leads to the function the next step is
// in.

// A BlockStep is one step of the reason a function blocks: a call of a
// function that blocks, in the function the step before leads to, or, as
// the last step, the operation that waits.
type BlockStep struct {
	Pos  token.Position
	What string
}

// A BlockingFunc is a function or function literal that blocks, and why.
type BlockingFunc struct {
	Name  string // the function's name (see blockFuncName); "" for a literal
	Pos   token.Position
	Steps []BlockStep
}

// Blocking returns the functions and function literals of the package
// pkgPath that block, in source order, each with the chain of calls that
// makes it block. The program must have its entries set (SetEntries).
func (p *Program) Blocking(pkgPath string) []BlockingFunc {
	x := &explainer{bodies: map[any]funcBody{}}
	var keys []any
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		own := pkg.PkgPath == pkgPath
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					fn, _ := info.Defs[n.Name].(*types.Func)
					if fn == nil {
						return false
					}
					x.bodies[fn] = funcBody{info, n.Body, n.Name.Pos(), funcJSImport(n)}
					if own {
						keys = append(keys, fn)
					}
				case *ast.FuncLit:
					x.bodies[n] = funcBody{info: info, body: n.Body, pos: n.Pos()}
					if own {
						keys = append(keys, n)
					}
				case *ast.RangeStmt:
					x.bodies[n] = funcBody{info: info, body: n.Body, pos: n.Pos()}
					if own {
						keys = append(keys, n)
					}
				}
				return true
			})
		}
	}
	var out []BlockingFunc
	for _, k := range keys {
		if !p.async[k] || p.syncOnly[k] {
			continue
		}
		b := x.bodies[k]
		f := BlockingFunc{Pos: p.Fset.Position(b.pos), Steps: p.whyChain(x, k)}
		if fn, ok := k.(*types.Func); ok {
			f.Name = blockFuncName(fn)
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Pos, out[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.Offset < b.Offset
	})
	return out
}

// An explainer holds what the explanations read besides the analysis.
type explainer struct {
	bodies map[any]funcBody // by unit key
}

// funcBody is the body of a declared function, a function literal or the
// yield function of a range-over-func statement.
type funcBody struct {
	info *types.Info
	body *ast.BlockStmt // nil for a function without a Go body
	pos  token.Pos      // of the function's name, the literal or the statement
	js   *jsImport      // of a JavaScript function a body-less function imports
}

// whyChain returns the steps that make the unit key block.
func (p *Program) whyChain(x *explainer, key any) []BlockStep {
	seen := map[any]bool{}
	var steps []BlockStep
	for key != nil && len(steps) < 100 {
		seen[key] = true
		st, next, ok := p.whyStep(x, key, seen)
		if !ok {
			break
		}
		steps = append(steps, st)
		if next != nil && seen[next] {
			break
		}
		key = next
	}
	return steps
}

// whyStep returns the first step of why the unit key blocks, and the unit the
// chain goes on in (nil at the end). It prefers a step that leads to a unit
// the chain has not been in, so that a recursion leads on to what blocks.
func (p *Program) whyStep(x *explainer, key any, seen map[any]bool) (BlockStep, any, bool) {
	pos := func(at token.Pos) token.Position { return p.Fset.Position(at) }
	if fn, ok := key.(*types.Func); ok {
		if t := p.linkTargets[fn]; t != nil && p.async[t] {
			return BlockStep{pos(fn.Pos()), "is linked (go:linkname) to " + blockFuncName(t)}, t, true
		}
	}
	b, ok := x.bodies[key]
	if !ok {
		return BlockStep{}, nil, false
	}
	if b.body == nil {
		what := "has no Go body"
		if b.js != nil {
			what = fmt.Sprintf("awaits the Promise that the JavaScript function %s of %q returns (//goesm:import ... await)", b.js.name, b.js.spec)
		}
		return BlockStep{pos(b.pos), what}, nil, true
	}
	info := b.info
	var (
		first  *BlockStep
		found  *BlockStep
		next   any
		inGo   = map[*ast.CallExpr]bool{}
		nowait = map[ast.Node]bool{}
	)
	// consider records a step: the first one that leads to a unit not seen
	// yet ends the walk.
	consider := func(at token.Pos, what string, to any) {
		st := BlockStep{pos(at), what}
		if first == nil {
			first = &st
		}
		if to == nil || !seen[to] {
			found, next = &st, to
		}
	}
	inspectLive(info, b.body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			return false // its own unit
		case *ast.GoStmt:
			inGo[n.Call] = true
		case *ast.SendStmt:
			if !nowait[n] {
				consider(n.Arrow, "sends on a channel", nil)
			}
		case *ast.UnaryExpr:
			if n.Op == token.ARROW && !nowait[n] {
				consider(n.OpPos, "receives from a channel", nil)
			}
		case *ast.SelectStmt:
			hasDefault := false
			for _, c := range n.Body.List {
				if c.(*ast.CommClause).Comm == nil {
					hasDefault = true
				}
			}
			if !hasDefault {
				consider(n.Select, "waits in a select statement", nil)
				break
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
				consider(n.For, "ranges over a channel", nil)
			case *types.Signature:
				u := &unit{encl: p.encls[n]}
				p.classifyFunc(info, n.X, u)
				if what, to, ok := p.whyBlocker(u, nil); ok {
					consider(n.X.Pos(), "ranges over a function: "+what, to)
				}
			}
		case *ast.CallExpr:
			if inGo[n] || !p.CallBlocksIn(info, n, nil) {
				break
			}
			u := &unit{encl: p.encls[n]}
			p.classifyCall(info, n, u, false)
			if what, to, ok := p.whyBlocker(u, n); ok {
				consider(n.Pos(), what, to)
			}
		}
		return true
	})
	if found != nil {
		return *found, next, true
	}
	if first != nil {
		return *first, nil, true
	}
	return BlockStep{pos(b.pos), "blocks"}, nil, true
}

// whyBlocker returns why the calls classified in u (one call, call, or the
// iterator call of a range statement when call is nil) block, and the unit
// the call leads to, following unitBlocksIn.
func (p *Program) whyBlocker(u *unit, call *ast.CallExpr) (string, any, bool) {
	if u.blocking && call != nil {
		if p.waitLocks[call] != nil {
			return "calls " + types.ExprString(call.Fun) + ", which waits while another goroutine holds the lock", nil, true
		}
		if p.pullCalls[call] {
			return "calls a function of iter.Pull, which waits for the iterator", nil, true
		}
		return "blocks", nil, true
	}
	for _, c := range u.callees {
		if p.async[c] {
			return "calls " + p.whyName(c), c, true
		}
	}
	for _, c := range u.varFns {
		if p.async[c] && (p.live == nil || p.live(c)) {
			return "calls a function variable that holds " + p.whyName(c), c, true
		}
	}
	for _, s := range u.sites {
		if !p.siteBlocks(s, nil) {
			continue
		}
		// s.fn may block only through what it is passed: the chain goes
		// on in s.fn, at its call of what this call passes.
		return "calls " + blockFuncName(s.fn), s.fn, true
	}
	param := "a function parameter"
	if call != nil {
		if pc := p.paramCalls[call]; pc.fn != nil {
			param = "its function parameter " + paramName(pc.fn, pc.idx)
		}
	}
	for _, c := range u.paramFns {
		if p.async[c] && (p.live == nil || p.live(c)) {
			return "calls " + param + ", which may be " + p.whyName(c), c, true
		}
	}
	for _, d := range u.paramDyn {
		if p.nonEsc[d.fn][d.idx] {
			if b := p.paramBlocker(d.fn, d.idx, d.sig, map[paramKey]bool{}); b != nil {
				return p.whyThrough("calls its function parameter "+paramName(d.fn, d.idx)+", which may be ", b)
			}
		} else if b := p.dynSigBlocker(d.sig, u, p.units); b != nil {
			return p.whyThrough("calls its function parameter "+paramName(d.fn, d.idx)+", which may be ", b)
		}
	}
	for _, s := range u.dynSigs {
		if b := p.dynSigBlocker(s, u, p.units); b != nil {
			return p.whyThrough("calls a function value of type "+types.TypeString(s, qualifier)+", which may be ", b)
		}
	}
	for _, c := range append(append([]ifaceCall{}, u.ifaceCalls...), u.paramIface...) {
		if m := p.ifaceCallBlocker(c.recv, c.fn, map[types.Type]bool{}); m != nil {
			return "calls the method " + c.fn.Name() + " of " + types.TypeString(c.recv, qualifier) + ", which may be " + blockFuncName(m), m, true
		}
	}
	return "", nil, false
}

// whyThrough describes a call that leads to the blocker b (see
// funcArgBlocker and dynSigBlocker).
func (p *Program) whyThrough(what string, b any) (string, any, bool) {
	switch b := b.(type) {
	case *types.Func, *ast.FuncLit, *ast.RangeStmt:
		return what + p.whyName(b), b, true
	case ifaceCall:
		if m := p.ifaceCallBlocker(b.recv, b.fn, map[types.Type]bool{}); m != nil {
			return what + "the method value " + types.TypeString(b.recv, qualifier) + "." + b.fn.Name() + ", which may be " + blockFuncName(m), m, true
		}
	case *ast.SelectorExpr:
		if p.waitLockVals[b] != nil {
			return what + "the method value " + types.ExprString(b) + " (" + p.Fset.Position(b.Pos()).String() + "), which waits while another goroutine holds the lock", nil, true
		}
	case ast.Node:
		return what + "a function that is not known where it is called (" + p.Fset.Position(b.Pos()).String() + ")", nil, true
	}
	return what + "a function that blocks", nil, true
}

// whyName names a unit: a function by its name, a literal by its position.
func (p *Program) whyName(key any) string {
	switch k := key.(type) {
	case *types.Func:
		return blockFuncName(k)
	case *ast.FuncLit:
		return "the function literal at " + p.Fset.Position(k.Pos()).String()
	case *ast.RangeStmt:
		return "the body of the range statement at " + p.Fset.Position(k.Pos()).String()
	}
	return fmt.Sprint(key)
}

// blockFuncName names a function as Go's error messages do: pkg.F, (*pkg.T).M,
// with the package's name, not its path.
func blockFuncName(fn *types.Func) string {
	fn = fn.Origin()
	sig := fn.Signature()
	if r := sig.Recv(); r != nil {
		return "(" + types.TypeString(r.Type(), qualifier) + ")." + fn.Name()
	}
	if fn.Pkg() == nil {
		return fn.Name()
	}
	return fn.Pkg().Name() + "." + fn.Name()
}

func qualifier(p *types.Package) string { return p.Name() }

// paramName is the name of the parameter idx of fn, or its index.
func paramName(fn *types.Func, idx int) string {
	ps := fn.Signature().Params()
	if idx < ps.Len() && ps.At(idx).Name() != "" && ps.At(idx).Name() != "_" {
		return ps.At(idx).Name()
	}
	return fmt.Sprintf("#%d", idx+1)
}
