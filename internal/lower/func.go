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
	hasDefer bool
	results  []string // JS references to the result variables, when materialised
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
		return fe.pe.qualify(obj.Pkg(), jsName(obj.Name()))
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
	switch sig.Results().Len() {
	case 0:
		return "void"
	case 1:
		return fe.ts(sig.Results().At(0).Type())
	}
	var ts []string
	for i := 0; i < sig.Results().Len(); i++ {
		ts = append(ts, fe.ts(sig.Results().At(i).Type()))
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
// place: assignment to v or its fields/array elements, &v, or a pointer
// method call on v. Used to decide whether a value receiver must be copied.
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
	// A value receiver is a copy in Go; copy only if the body could tell.
	if recv := sig.Recv(); recv != nil && recvList != nil && isAggregate(recv.Type()) && !fe.boxed(recv) {
		if _, isPtr := recv.Type().(*types.Pointer); !isPtr {
			var rv *types.Var
			for _, f := range recvList.List {
				for _, id := range f.Names {
					rv, _ = fe.info.Defs[id].(*types.Var)
				}
			}
			if rv != nil && fe.mutatesVar(body, rv) {
				n := fe.nameOf(rv)
				w.ln("%s = %s;", n, fe.pe.copyExpr(n, rv.Type(), fe.tp))
			}
		}
	}

	fe.hasDefer = containsDefer(body)
	named := sig.Results().Len() > 0 && sig.Results().At(0).Name() != ""
	fe.results = nil
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
			if fe.boxed(r) {
				init = "$rt.cell(" + init + ")"
				ref = n + ".v"
			}
			w.ln("let %s: %s = %s;", n, fe.ts(r.Type()), init)
			fe.results = append(fe.results, ref)
		}
	}
	if !fe.hasDefer {
		fe.stmts(body.List)
		return
	}
	w.ln("const $d = new $rt.Defers();")
	w.ln("$body: try {")
	w.indent++
	fe.stmts(body.List)
	w.indent--
	if fe.async {
		w.ln("} catch ($e) { $d.fail($e); } finally { await $d.runAsync(); }")
	} else {
		w.ln("} catch ($e) { $d.fail($e); } finally { $d.run(); }")
	}
	if r := fe.resultsExpr(); r != "" {
		w.ln("return %s;", r)
	}
}

func (fe *funcEmitter) resultsExpr() string {
	switch len(fe.results) {
	case 0:
		return ""
	case 1:
		return fe.results[0]
	}
	return "[" + strings.Join(fe.results, ", ") + "]"
}

// funcLit lowers a function literal to a JS arrow function.
func (fe *funcEmitter) funcLit(lit *ast.FuncLit) string {
	sig := fe.info.TypeOf(lit).(*types.Signature)
	w := newRawWriter(fe.pe.tab)
	w.indent = fe.w.indent + 1
	c := fe.child(w, sig)
	c.async = fe.pe.prog.LitAsync(lit)
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
	fe.w.ln("const %s = %s;", t, s)
	return t
}

func (fe *funcEmitter) forceTmp(s string) string {
	t := fe.tmp()
	fe.w.ln("const %s = %s;", t, s)
	return t
}
