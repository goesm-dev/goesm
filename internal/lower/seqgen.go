package lower

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
)

// Sequences as generators.
//
// iter.Pull runs a sequence on a coroutine, which goesm makes a goroutine:
// each value pulled takes two switches through Promises and the event loop,
// where a JS generator takes a call. A function literal that is a sequence
// (func(yield func(V) bool) or func(yield func(K, V) bool)), calls yield only
// directly and blocks on nothing else is therefore also lowered as a
// generator, which the literal carries ($rt.withSeqGen) and Pull steps
// synchronously (internal/natives/patch/iter). Each call of yield becomes a
// generator yield of its boxed argument (a Seq2's key goes in the state's
// k), which evaluates to the result next passes in; once stop has set the state's d, yield returns false at
// once, as Go's does after stop. The generator is only made in programs
// that call Pull or Pull2, and dropped when its body would need an await.

// seqYield returns the yield parameter of lit if lit is a sequence that
// calls it only directly, outside nested functions, go and defer statements
// and the init and post statements of for loops (which may become nested
// functions), or nil.
func seqYield(info *types.Info, lit *ast.FuncLit, sig *types.Signature) *types.Var {
	if sig.Params().Len() != 1 || sig.Results().Len() != 0 || len(lit.Type.Params.List) != 1 || len(lit.Type.Params.List[0].Names) != 1 {
		return nil
	}
	ys, ok := under(sig.Params().At(0).Type()).(*types.Signature)
	if !ok || ys.Results().Len() != 1 || ys.Params().Len() < 1 || ys.Params().Len() > 2 || ys.Variadic() {
		return nil
	}
	if b, ok := under(ys.Results().At(0).Type()).(*types.Basic); !ok || b.Kind() != types.Bool {
		return nil
	}
	yv, _ := info.Defs[lit.Type.Params.List[0].Names[0]].(*types.Var)
	if yv == nil || yv.Name() == "_" || containsGoto(lit.Body) {
		return nil
	}
	ok = true
	direct := map[*ast.Ident]bool{} // the uses of yv called in safe places
	var walk func(n ast.Node, safe bool)
	walk = func(n ast.Node, safe bool) {
		ast.Inspect(n, func(n ast.Node) bool {
			if !ok {
				return false
			}
			switch n := n.(type) {
			case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt:
				walk2 := func(n ast.Node) {
					ast.Inspect(n, func(n ast.Node) bool {
						if id, isId := n.(*ast.Ident); isId && info.Uses[id] == yv {
							ok = false
						}
						return ok
					})
				}
				walk2(n)
				return false
			case *ast.ForStmt:
				if n.Init != nil {
					walk(n.Init, false)
				}
				if n.Cond != nil {
					walk(n.Cond, safe)
				}
				if n.Post != nil {
					walk(n.Post, false)
				}
				walk(n.Body, safe)
				return false
			case *ast.RangeStmt:
				if _, isFunc := under(info.TypeOf(n.X)).(*types.Signature); isFunc {
					walk(n.Body, false) // the loop body is a function
					walk(n.X, safe)
					return false
				}
			case *ast.CallExpr:
				if id, isId := unparen(n.Fun).(*ast.Ident); isId && info.Uses[id] == yv {
					if !safe || n.Ellipsis.IsValid() {
						ok = false
						return false
					}
					direct[id] = true
				}
			case *ast.Ident:
				if info.Uses[n] == yv && !direct[n] {
					ok = false
				}
			}
			return true
		})
	}
	walk(lit.Body, true)
	if !ok || len(direct) == 0 {
		return nil
	}
	return yv
}

// seqGenerator returns the generator of lit (see seqYield), or "".
func (fe *funcEmitter) seqGenerator(lit *ast.FuncLit, sig *types.Signature) string {
	if !fe.pe.prog.usesPull {
		return ""
	}
	yv := seqYield(fe.info, lit, sig)
	if yv == nil {
		return ""
	}
	diags := len(fe.pe.prog.Diags) // reported by the function itself
	w := newRawWriter(fe.pe.tab)
	w.indent = fe.w.indent + 1
	c := fe.child(w, sig)
	c.recoverTok = fe.pe.litRecoverTok(lit)
	c.syncOnly = fe.pe.prog.SyncOnly(lit)
	c.genYield = yv
	c.genState = fe.declareName("$gs")
	c.funcBody(nil, lit.Type, lit.Body, sig)
	fe.pe.prog.Diags = fe.pe.prog.Diags[:diags]
	body := w.String()
	if strings.Contains(stripMarks(body), "await") {
		return ""
	}
	return fmt.Sprintf("function* (%s: any): any {\n%s%s}", c.genState, body, strings.Repeat("  ", fe.w.indent))
}

// genYieldCall lowers a call of the yield parameter in a generator (see
// seqGenerator).
func (fe *funcEmitter) genYieldCall(e *ast.CallExpr) string {
	ys := under(fe.genYield.Type()).(*types.Signature)
	anyT := types.Universe.Lookup("any").Type()
	var vals []string
	for i, a := range e.Args {
		pt := ys.Params().At(i).Type()
		vals = append(vals, fe.convertCopy(fe.valueOf(a, pt), pt, anyT))
	}
	// The arguments are evaluated also when yield returns false at once.
	t := fe.declareName("$y")
	fe.temps = append(fe.temps, t)
	k := ""
	if len(vals) == 2 {
		k = fe.genState + ".k = " + vals[0] + ", "
		vals = vals[1:]
	}
	return fmt.Sprintf("%s(%s%s = %s, %s.d ? false : (yield %s))", fe.mark(e), k, t, vals[0], fe.genState, t)
}

// usesPullFuncs reports whether a package other than iter refers to
// iter.Pull or Pull2.
func (p *Program) usesPullFuncs() bool {
	for _, pkg := range p.Pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && fn.Pkg() != pkg.Types && coroutineFuncs[fn.Origin().FullName()] {
				return true
			}
		}
	}
	return false
}
