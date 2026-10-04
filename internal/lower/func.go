package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"go/version"
	"regexp"
	"strings"
)

// funcEmitter lowers one function body. Nested function literals get a child
// emitter that shares the naming state of the enclosing declaration, so every
// Go object in a declaration has one unique JS name (Go's shadowing never
// has to be reproduced with JS scoping rules).
type funcEmitter struct {
	pe   *pkgEmitter
	info *types.Info
	w    *writer
	file *ast.File

	names    map[types.Object]string
	used     map[string]bool
	override map[ast.Expr]string
	tmpN     *int

	tp       tpScope
	sig      *types.Signature
	async    bool
	syncOnly bool // channel operations must complete at once (natives.Sync)
	hasDefer bool
	results  []string // JS references to the result variables, when materialised
	resultTs []types.Type
	named    bool                     // results are named Go variables (their address may escape)
	gotos    map[*ast.BranchStmt]bool // forward gotos, lowered to labelled breaks
	// Goto state machines (see gotoMachine): the targets of their labels,
	// the variables declared before them, and the enclosing breakable
	// statements, whose unlabelled break and continue a machine would capture.
	gotoTargets map[types.Object]gotoTarget
	hoisted     map[*types.Var]bool
	// sharedRangeVars are the variables of range clauses in files before Go
	// 1.22, declared once around their loop.
	sharedRangeVars map[*types.Var]bool
	breakables      []breakable
	rangeFn         *rangeFuncCtx // the range-over-func body being lowered
	// recoverTok identifies the function being lowered to recover(), which
	// only recovers when called by the deferred function itself.
	recoverTok string
}

func (pe *pkgEmitter) newFuncEmitter(w *writer, sig *types.Signature) *funcEmitter {
	n := 0
	return &funcEmitter{
		pe:       pe,
		info:     pe.info,
		w:        w,
		names:    map[types.Object]string{},
		used:     map[string]bool{},
		override: map[ast.Expr]string{},
		tmpN:     &n,
		sig:      sig,
	}
}

func (fe *funcEmitter) child(w *writer, sig *types.Signature) *funcEmitter {
	return &funcEmitter{
		pe: fe.pe, info: fe.info, w: w, file: fe.file,
		names: fe.names, used: fe.used, override: fe.override, tmpN: fe.tmpN,
		tp: fe.tp, sig: sig,
	}
}

func (fe *funcEmitter) errorf(pos token.Pos, format string, args ...any) {
	fe.pe.errorf(pos, format, args...)
}

func (fe *funcEmitter) mark(n ast.Node) string {
	if n.Pos().IsValid() {
		fe.pe.lastPos = n.Pos()
	}
	return fe.pe.tab.mark(n.Pos())
}

func (fe *funcEmitter) tmp() string {
	*fe.tmpN++
	return fmt.Sprintf("$%d", *fe.tmpN)
}

func (fe *funcEmitter) declareName(base string) string {
	name := base
	for i := 1; fe.used[name] || fe.pe.reserved[name]; i++ {
		name = fmt.Sprintf("%s$%d", base, i)
	}
	fe.used[name] = true
	return name
}

func (fe *funcEmitter) declare(obj types.Object) string {
	if n, ok := fe.names[obj]; ok {
		return n
	}
	base := jsName(obj.Name())
	if obj.Name() == "_" || obj.Name() == "" {
		base = "$_"
	}
	n := fe.declareName(base)
	fe.names[obj] = n
	return n
}

// nameOf returns the JS reference for a Go object.
func (fe *funcEmitter) nameOf(obj types.Object) string {
	if n, ok := fe.names[obj]; ok {
		return n
	}
	if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
		if obj.Pkg() != fe.pe.pkg.Types {
			// Exported under its Go name (math.NaN), which is a valid
			// property name even where it is not a valid identifier.
			return fe.pe.qualify(obj.Pkg(), obj.Name())
		}
		return jsName(obj.Name())
	}
	return fe.declare(obj)
}

func (fe *funcEmitter) boxed(v *types.Var) bool { return fe.pe.prog.boxed[v] }

func (fe *funcEmitter) varRef(v *types.Var) string {
	n := fe.nameOf(v)
	if fe.boxed(v) {
		return n + ".v"
	}
	return n
}

func (fe *funcEmitter) goVersionAtLeast(v string) bool {
	if fe.file == nil {
		return true
	}
	fv := fe.info.FileVersions[fe.file]
	if fv == "" {
		return true
	}
	return version.Compare(fv, v) >= 0
}

func (fe *funcEmitter) desc(t types.Type) string { return fe.pe.typeDesc(t, fe.tp) }
func (fe *funcEmitter) zero(t types.Type) string { return fe.pe.zeroOf(t, fe.tp) }
func (fe *funcEmitter) zeroFn(t types.Type) string {
	return fe.pe.zeroFn(t, fe.tp)
}
func (fe *funcEmitter) ts(t types.Type) string { return fe.pe.tsType(t, fe.tp) }

// paramList declares parameters and returns their JS declarations.
func (fe *funcEmitter) paramList(fields *ast.FieldList, recv *types.Var) []string {
	var out []string
	if fields == nil {
		return nil
	}
	for _, f := range fields.List {
		t := fe.info.TypeOf(f.Type)
		if ell, ok := f.Type.(*ast.Ellipsis); ok {
			t = types.NewSlice(fe.info.TypeOf(ell.Elt))
		}
		if len(f.Names) == 0 {
			name := fe.declareName("$p")
			if recv != nil {
				fe.names[recv] = name
			}
			out = append(out, name+": "+fe.ts(t))
			continue
		}
		for _, id := range f.Names {
			obj := fe.info.Defs[id]
			var name string
			if obj == nil {
				name = fe.declareName("$p")
			} else {
				name = fe.declare(obj)
			}
			out = append(out, name+": "+fe.ts(t))
		}
	}
	return out
}

func (fe *funcEmitter) resultTSType(sig *types.Signature) string {
	return fe.pe.resultTSType(sig, fe.tp)
}

// resultTSType is the TypeScript result type of a function: void, the
// type, or a tuple for several results.
func (pe *pkgEmitter) resultTSType(sig *types.Signature, tp tpScope) string {
	switch sig.Results().Len() {
	case 0:
		return "void"
	case 1:
		return pe.tsType(sig.Results().At(0).Type(), tp)
	}
	var ts []string
	for i := 0; i < sig.Results().Len(); i++ {
		ts = append(ts, pe.tsType(sig.Results().At(i).Type(), tp))
	}
	return "[" + strings.Join(ts, ", ") + "]"
}

func containsDefer(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeferStmt:
			found = true
		}
		return !found
	})
	return found
}

// mutatesVar reports whether body may modify (part of) the value of v in
// place, or keep a reference to it beyond the call: assignment to v or its
// fields/array elements, &v, a pointer method call on v, slicing an array
// in v, or a reference to v from a function literal. Used to decide whether
// a value receiver must be copied (otherwise it aliases the caller's value).
func (fe *funcEmitter) mutatesVar(body *ast.BlockStmt, v *types.Var) bool {
	root := func(e ast.Expr) bool {
		for {
			switch x := unparen(e).(type) {
			case *ast.Ident:
				return fe.info.Uses[x] == v
			case *ast.SelectorExpr:
				if _, ok := fe.info.Selections[x]; !ok {
					return false
				}
				if _, isPtr := under(fe.info.TypeOf(x.X)).(*types.Pointer); isPtr {
					return false
				}
				e = x.X
			case *ast.IndexExpr:
				if _, ok := under(fe.info.TypeOf(x.X)).(*types.Array); !ok {
					return false
				}
				e = x.X
			default:
				return false
			}
		}
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			ast.Inspect(n.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && fe.info.Uses[id] == v {
					found = true
				}
				return !found
			})
		case *ast.SliceExpr:
			found = found || root(n.X)
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				if root(l) {
					found = true
				}
			}
		case *ast.IncDecStmt:
			found = found || root(n.X)
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				found = found || (n.Key != nil && root(n.Key)) || (n.Value != nil && root(n.Value))
			}
		case *ast.UnaryExpr:
			found = found || (n.Op == token.AND && root(n.X))
		case *ast.SelectorExpr:
			if sel, ok := fe.info.Selections[n]; ok && sel.Kind() == types.MethodVal {
				if _, ptr := sel.Obj().(*types.Func).Signature().Recv().Type().(*types.Pointer); ptr && root(n.X) {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// funcBody emits the statements of a function (the caller wrote the
// signature line).
func (fe *funcEmitter) funcBody(recvList *ast.FieldList, ftype *ast.FuncType, body *ast.BlockStmt, sig *types.Signature) {
	w := fe.w
	if fe.async && fe.pe.prog.TracksGoroutines {
		w.ln("const $g = $rt.getG();") // the goroutine to restore after each await
	}
	// Parameters whose address is taken live in cells.
	box := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			for _, id := range f.Names {
				if v, ok := fe.info.Defs[id].(*types.Var); ok && fe.boxed(v) {
					n := fe.nameOf(v)
					w.ln("%s = $rt.cell(%s) as any;", n, n)
				}
			}
		}
	}
	box(recvList)
	box(ftype.Params)
	if fe.containsRecover(body) {
		// Whether this call is the deferred call itself, the one frame in
		// which recover() recovers (see recover in runtime/src/panic.ts).
		w.ln("const $rf = $rt.recoverFrame(%s);", jsString(fe.recoverTok))
	}
	// A value receiver is a copy in Go; copy only if the body could tell.
	if recv := sig.Recv(); recv != nil && recvList != nil && isAggregate(recv.Type()) && !fe.boxed(recv) {
		if _, isPtr := recv.Type().(*types.Pointer); !isPtr {
			var rv *types.Var
			for _, f := range recvList.List {
				for _, id := range f.Names {
					rv, _ = fe.info.Defs[id].(*types.Var)
				}
			}
			// An async method may observe the caller's changes made while
			// it is blocked.
			if rv != nil && (fe.async || fe.mutatesVar(body, rv)) {
				n := fe.nameOf(rv)
				w.ln("%s = %s;", n, fe.pe.copyExpr(n, rv.Type(), fe.tp))
			}
		}
	}

	fe.hasDefer = containsDefer(body)
	named := sig.Results().Len() > 0 && sig.Results().At(0).Name() != ""
	fe.results, fe.resultTs, fe.named = nil, nil, named
	if named || fe.hasDefer {
		for i := 0; i < sig.Results().Len(); i++ {
			r := sig.Results().At(i)
			var n string
			if named && r.Name() != "_" {
				n = fe.declare(r)
			} else {
				n = fe.declareName(fmt.Sprintf("$r%d", i))
			}
			init := fe.zero(r.Type())
			ref := n
			tsT := fe.ts(r.Type())
			if fe.boxed(r) {
				init = "$rt.cell(" + init + ")"
				ref = n + ".v"
				tsT = "$rt.Cell<" + tsT + ">"
			}
			w.ln("let %s: %s = %s;", n, tsT, init)
			fe.results = append(fe.results, ref)
			fe.resultTs = append(fe.resultTs, r.Type())
		}
	}
	if !fe.hasDefer {
		fe.stmts(body.List)
		if sig.Results().Len() > 0 && fe.mayFallOff(body.List) {
			// go/types guarantees a terminating statement; TypeScript's
			// flow analysis cannot always see it (a switch or type switch
			// whose every case returns).
			// globalThis: the package may declare its own Error.
			w.ln("throw new globalThis.Error(\"goesm: unreachable\");")
		}
		return
	}
	w.ln("const $d = new $rt.Defers();")
	if containsReturn(body) {
		w.ln("$body: try {") // returns break out to run the deferred calls
	} else {
		w.ln("try {")
	}
	w.indent++
	fe.stmts(body.List)
	w.indent--
	if fe.async {
		if fe.pe.prog.TracksGoroutines {
			w.ln("} catch ($e) { $rt.resumeG($g, 0); $d.fail($e); } finally { %s; }", fe.await("$d.runAsync()"))
		} else {
			w.ln("} catch ($e) { $d.fail($e); } finally { await $d.runAsync(); }")
		}
	} else {
		w.ln("} catch ($e) { $d.fail($e); } finally { $d.run(); }")
	}
	if r := fe.resultsExpr(); r != "" {
		w.ln("return %s;", r)
	}
}

// resultsExpr returns the result variables as the function's return value.
// Named aggregate results are copied out: a pointer to the variable may
// have escaped and must not alias the caller's value.
func (fe *funcEmitter) resultsExpr() string {
	var rs []string
	for i, r := range fe.results {
		if fe.named && isAggregate(fe.resultTs[i]) {
			r = fe.pe.copyExpr(r, fe.resultTs[i], fe.tp)
		}
		rs = append(rs, r)
	}
	switch len(rs) {
	case 0:
		return ""
	case 1:
		return rs[0]
	}
	return "[" + strings.Join(rs, ", ") + "]"
}

// setResults stores return values into the result variables (named
// aggregates in place, so escaped pointers observe them).
func (fe *funcEmitter) setResults(m string, vals []string) {
	if len(vals) > 1 {
		for i, v := range vals {
			vals[i] = fe.forceTmp(v)
		}
	}
	for i, v := range vals {
		if stripMarks(v) == fe.results[i] {
			continue // return of the named result itself
		}
		if fe.named && isAggregate(fe.resultTs[i]) {
			fe.w.ln("%s%s;", m, fe.aggregateSet(fe.results[i], fe.resultTs[i], v))
		} else {
			fe.w.ln("%s%s = %s;", m, fe.results[i], v)
		}
	}
}

// funcLit lowers a function literal to a JS arrow function.
func (fe *funcEmitter) funcLit(lit *ast.FuncLit) string {
	sig := fe.info.TypeOf(lit).(*types.Signature)
	w := newRawWriter(fe.pe.tab)
	w.indent = fe.w.indent + 1
	c := fe.child(w, sig)
	c.recoverTok = litRecoverTok(lit)
	c.async = fe.pe.prog.LitAsync(lit)
	c.syncOnly = fe.pe.prog.SyncOnly(lit)
	params := c.paramList(lit.Type.Params, nil)
	c.funcBody(nil, lit.Type, lit.Body, sig)
	prefix := ""
	ret := c.resultTSType(sig)
	if c.async {
		prefix = "async "
		ret = "Promise<" + ret + ">"
	}
	return fmt.Sprintf("%s%s(%s): %s => {\n%s%s}", fe.mark(lit), prefix, strings.Join(params, ", "), ret, w.String(), strings.Repeat("  ", fe.w.indent))
}

var simpleRef = regexp.MustCompile(`^[\w$]+(\.[\w$]+)*$`)

// jsLiteral matches numeric and string literals, which never change.
var jsLiteral = regexp.MustCompile(`^(\(?-?[0-9][0-9a-fA-Fxob._e+-]*\)?|"([^"\\]|\\.)*")$`)

// stable returns s, or a temporary holding s if s is not a plain reference.
func (fe *funcEmitter) stable(s string) string {
	if simpleRef.MatchString(stripMarks(s)) {
		return s
	}
	t := fe.tmp()
	// Temporaries are internal: typed any so that a constant tag or an
	// untyped tuple does not narrow what later code may do with them.
	fe.w.ln("const %s: any = %s;", t, s)
	return t
}

func (fe *funcEmitter) forceTmp(s string) string {
	t := fe.tmp()
	// Temporaries are internal: typed any so that a constant tag or an
	// untyped tuple does not narrow what later code may do with them.
	fe.w.ln("const %s: any = %s;", t, s)
	return t
}

// mayFallOff reports whether TypeScript may consider the end of a lowered
// function body reachable although Go's terminating-statement rule holds:
// the body does not end in a return, a panic call or an infinite for loop.
func (fe *funcEmitter) mayFallOff(list []ast.Stmt) bool {
	if len(list) == 0 {
		return true
	}
	switch s := list[len(list)-1].(type) {
	case *ast.ReturnStmt:
		return false
	case *ast.ForStmt:
		return s.Cond != nil
	case *ast.ExprStmt:
		if call, ok := unparen(s.X).(*ast.CallExpr); ok {
			if id, ok := unparen(call.Fun).(*ast.Ident); ok {
				if b, ok := fe.info.Uses[id].(*types.Builtin); ok && b.Name() == "panic" {
					return false
				}
			}
		}
	case *ast.BlockStmt:
		return fe.mayFallOff(s.List)
	case *ast.IfStmt:
		return s.Else == nil || fe.mayFallOff(s.Body.List) || fe.mayFallOff([]ast.Stmt{s.Else})
	}
	return true
}

// recvExpr lowers a receive from ch to a [value, ok] pair.
func (fe *funcEmitter) recvExpr(ch string) string {
	if fe.syncOnly {
		return "$rt.recvNow(" + ch + ")"
	}
	return "(" + fe.await("$rt.recv("+ch+")") + ")"
}

// await awaits the JS expression s. When the program tracks goroutines, the
// running goroutine is restored once the await resumes.
func (fe *funcEmitter) await(s string) string {
	if fe.sig == nil {
		return fe.pe.prog.awaitMain(s) // a package variable's initializer
	}
	if fe.pe.prog.TracksGoroutines {
		return "$rt.resumeG($g, await " + s + ")"
	}
	return "await " + s
}

// awaitMain awaits s at the top level of a module, on the main goroutine.
func (p *Program) awaitMain(s string) string {
	if p.TracksGoroutines {
		return "$rt.resumeG($rt.mainG, await " + s + ")"
	}
	return "await " + s
}

// containsRecover reports whether body calls recover itself (not in a
// function literal).
func (fe *funcEmitter) containsRecover(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if id, ok := unparen(n.Fun).(*ast.Ident); ok {
				if b, ok := fe.info.Uses[id].(*types.Builtin); ok && b.Name() == "recover" {
					found = true
				}
			}
		}
		return !found
	})
	return found
}
