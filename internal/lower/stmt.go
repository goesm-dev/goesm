package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

func (fe *funcEmitter) stmts(list []ast.Stmt) {
	for _, s := range list {
		fe.stmt(s, "")
	}
}

func (fe *funcEmitter) block(list []ast.Stmt) {
	fe.w.indent++
	fe.stmts(list)
	fe.w.indent--
}

func labelPrefix(label string) string {
	if label == "" {
		return ""
	}
	return label + ": "
}

func (fe *funcEmitter) stmt(s ast.Stmt, label string) {
	w := fe.w
	m := fe.mark(s)
	switch s := s.(type) {
	case *ast.EmptyStmt:
	case *ast.ExprStmt:
		w.ln("%s%s;", m, fe.expr(s.X))
	case *ast.DeclStmt:
		fe.declStmt(s)
	case *ast.AssignStmt:
		fe.assign(s)
	case *ast.IncDecStmt:
		lv := fe.lvalue(s.X, true)
		t := fe.info.TypeOf(s.X)
		op := token.ADD
		if s.Tok == token.DEC {
			op = token.SUB
		}
		w.ln("%s%s;", m, lv.set(fe.arith(op, lv.get, "1", t)))
	case *ast.BlockStmt:
		w.ln("%s%s{", m, labelPrefix(label))
		fe.block(s.List)
		w.ln("}")
	case *ast.LabeledStmt:
		name := fe.labelName(s.Label)
		switch s.Stmt.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.BlockStmt:
			fe.stmt(s.Stmt, name)
		default:
			w.ln("%s: {", name)
			fe.block([]ast.Stmt{s.Stmt})
			w.ln("}")
		}
	case *ast.IfStmt:
		fe.ifStmt(s, label)
	case *ast.ForStmt:
		fe.forStmt(s, label)
	case *ast.RangeStmt:
		fe.rangeStmt(s, label)
	case *ast.SwitchStmt:
		fe.switchStmt(s, label)
	case *ast.TypeSwitchStmt:
		fe.typeSwitchStmt(s, label)
	case *ast.SelectStmt:
		fe.selectStmt(s, label)
	case *ast.ReturnStmt:
		fe.returnStmt(s)
	case *ast.BranchStmt:
		switch s.Tok {
		case token.BREAK, token.CONTINUE:
			kw := "break"
			if s.Tok == token.CONTINUE {
				kw = "continue"
			}
			if s.Label != nil {
				w.ln("%s%s %s;", m, kw, fe.labelName(s.Label))
			} else {
				w.ln("%s%s;", m, kw)
			}
		case token.FALLTHROUGH:
			// handled by switchStmt: the JS case is emitted without break
		case token.GOTO:
			fe.errorf(s.Pos(), "goto is not supported yet")
		}
	case *ast.GoStmt:
		closure := fe.deferredCall(s.Call)
		w.ln("%s$rt.go(%s);", m, closure)
	case *ast.DeferStmt:
		closure := fe.deferredCall(s.Call)
		w.ln("%s$d.defer(%s);", m, closure)
	case *ast.SendStmt:
		ch := fe.expr(s.Chan)
		elem := under(fe.info.TypeOf(s.Chan)).(*types.Chan).Elem()
		w.ln("%sawait $rt.send(%s, %s);", m, ch, fe.valueOf(s.Value, elem))
	default:
		fe.errorf(s.Pos(), "unsupported statement %T", s)
	}
}

func (fe *funcEmitter) labelName(id *ast.Ident) string { return "L$" + id.Name }

func (fe *funcEmitter) declStmt(s *ast.DeclStmt) {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return // const and type declarations need no code (types are hoisted)
	}
	for _, spec := range gd.Specs {
		vs := spec.(*ast.ValueSpec)
		var vars []*types.Var
		for _, id := range vs.Names {
			v, _ := fe.info.Defs[id].(*types.Var)
			vars = append(vars, v)
		}
		m := fe.mark(vs)
		switch {
		case len(vs.Values) == 0:
			for _, v := range vars {
				fe.defineVar(m, v, fe.zero(v.Type()))
			}
		case len(vs.Values) == len(vars):
			vals := make([]string, len(vars))
			for i, v := range vars {
				vals[i] = fe.valueOf(vs.Values[i], v.Type())
			}
			if len(vars) > 1 {
				for i := range vals {
					vals[i] = fe.forceTmp(vals[i])
				}
			}
			for i, v := range vars {
				fe.defineVar(m, v, vals[i])
			}
		default: // tuple
			t := fe.forceTmp(fe.expr(vs.Values[0]))
			tt := fe.info.TypeOf(vs.Values[0])
			for i, v := range vars {
				fe.defineVar(m, v, fe.convert(fmt.Sprintf("%s[%d]", t, i), tupleAt(tt, i), v.Type()))
			}
		}
	}
}

// defineVar declares a new local variable with an initial value.
func (fe *funcEmitter) defineVar(m string, v *types.Var, init string) {
	if v == nil || v.Name() == "_" {
		fe.w.ln("%s%s;", m, init)
		return
	}
	n := fe.declare(v)
	if fe.boxed(v) {
		fe.w.ln("%slet %s = $rt.cell(%s);", m, n, init)
		return
	}
	fe.w.ln("%slet %s: %s = %s;", m, n, fe.ts(v.Type()), init)
}

func tupleAt(t types.Type, i int) types.Type {
	if tt, ok := t.(*types.Tuple); ok {
		return tt.At(i).Type()
	}
	return t
}

// commaOk returns a tuple-valued JS expression for v, ok forms, or "".
func (fe *funcEmitter) commaOk(e ast.Expr) (string, types.Type, bool) {
	switch x := unparen(e).(type) {
	case *ast.IndexExpr:
		if mt, ok := under(fe.info.TypeOf(x.X)).(*types.Map); ok {
			return fmt.Sprintf("%s$rt.mapLookup(%s, %s, %s)", fe.mark(x), fe.expr(x.X), fe.valueOf(x.Index, mt.Key()), fe.zeroFn(mt.Elem())),
				types.NewTuple(types.NewVar(0, nil, "", mt.Elem()), types.NewVar(0, nil, "", types.Typ[types.Bool])), true
		}
	case *ast.TypeAssertExpr:
		t := fe.info.TypeOf(x.Type)
		return fmt.Sprintf("%s$rt.assertOk(%s, %s)", fe.mark(x), fe.expr(x.X), fe.desc(t)),
			types.NewTuple(types.NewVar(0, nil, "", t), types.NewVar(0, nil, "", types.Typ[types.Bool])), true
	case *ast.UnaryExpr:
		if x.Op == token.ARROW {
			elem := under(fe.info.TypeOf(x.X)).(*types.Chan).Elem()
			return fmt.Sprintf("%s(await $rt.recv(%s))", fe.mark(x), fe.expr(x.X)),
				types.NewTuple(types.NewVar(0, nil, "", elem), types.NewVar(0, nil, "", types.Typ[types.Bool])), true
		}
	}
	return "", nil, false
}

func (fe *funcEmitter) assign(s *ast.AssignStmt) {
	w := fe.w
	m := fe.mark(s)
	if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
		// op-assign: x op= y
		lv := fe.lvalue(s.Lhs[0], true)
		op := opAssign[s.Tok]
		t := fe.info.TypeOf(s.Lhs[0])
		var val string
		if op == token.SHL || op == token.SHR {
			val = fe.shift(op, lv.get, fe.expr(s.Rhs[0]), t)
		} else {
			val = fe.arith(op, lv.get, fe.valueOf(s.Rhs[0], t), t)
		}
		w.ln("%s%s;", m, lv.set(val))
		return
	}

	// Tuple-valued right-hand side: f() or comma-ok forms.
	if len(s.Lhs) > 1 && len(s.Rhs) == 1 {
		var src string
		var tt types.Type
		if e, t, ok := fe.commaOk(s.Rhs[0]); ok {
			src, tt = e, t
		} else {
			src, tt = fe.expr(s.Rhs[0]), fe.info.TypeOf(s.Rhs[0])
		}
		// Go evaluates the target operands (a[f()]) before the call.
		lvs := make([]*lvalue, len(s.Lhs))
		if s.Tok == token.ASSIGN {
			for i, l := range s.Lhs {
				lv := fe.lvalue(l, true)
				lvs[i] = &lv
			}
		}
		tmp := fe.forceTmp(m + src)
		for i, l := range s.Lhs {
			val := fe.convertCopy(fmt.Sprintf("%s[%d]", tmp, i), tupleAt(tt, i), fe.lhsType(l, tupleAt(tt, i)))
			if lvs[i] != nil {
				w.ln("%s;", lvs[i].set(val))
				continue
			}
			fe.assignOne(s.Tok, l, val, "")
		}
		return
	}

	if len(s.Lhs) == 1 {
		fe.assignOne(s.Tok, s.Lhs[0], fe.valueOf(s.Rhs[0], fe.lhsType(s.Lhs[0], fe.info.TypeOf(s.Rhs[0]))), m)
		return
	}

	// Parallel assignment: evaluate left operands and all right-hand sides
	// first, then assign left to right.
	type target struct {
		e  ast.Expr
		lv lvalue
	}
	var targets []target
	for _, l := range s.Lhs {
		if s.Tok == token.DEFINE {
			targets = append(targets, target{e: l})
			continue
		}
		targets = append(targets, target{e: l, lv: fe.lvalue(l, true)})
	}
	vals := make([]string, len(s.Rhs))
	for i, r := range s.Rhs {
		vals[i] = fe.forceTmp(fe.valueOf(r, fe.lhsType(s.Lhs[i], fe.info.TypeOf(r))))
	}
	for i, t := range targets {
		if s.Tok == token.DEFINE {
			fe.assignOne(s.Tok, t.e, vals[i], "")
			continue
		}
		w.ln("%s;", t.lv.set(vals[i]))
	}
}

var opAssign = map[token.Token]token.Token{
	token.ADD_ASSIGN: token.ADD, token.SUB_ASSIGN: token.SUB, token.MUL_ASSIGN: token.MUL,
	token.QUO_ASSIGN: token.QUO, token.REM_ASSIGN: token.REM, token.AND_ASSIGN: token.AND,
	token.OR_ASSIGN: token.OR, token.XOR_ASSIGN: token.XOR, token.SHL_ASSIGN: token.SHL,
	token.SHR_ASSIGN: token.SHR, token.AND_NOT_ASSIGN: token.AND_NOT,
}

// lhsType returns the type of an assignment target (or fallback for blank).
func (fe *funcEmitter) lhsType(l ast.Expr, fallback types.Type) types.Type {
	if id, ok := unparen(l).(*ast.Ident); ok && id.Name == "_" {
		return fallback
	}
	if t := fe.info.TypeOf(l); t != nil {
		return t
	}
	return fallback
}

// assignOne assigns an already lowered value to one target.
func (fe *funcEmitter) assignOne(tok token.Token, l ast.Expr, val, m string) {
	if id, ok := unparen(l).(*ast.Ident); ok {
		if id.Name == "_" {
			fe.w.ln("%s%s;", m, val)
			return
		}
		if tok == token.DEFINE {
			if v, ok := fe.info.Defs[id].(*types.Var); ok {
				fe.defineVar(m, v, val)
				return
			}
		}
	}
	lv := fe.lvalue(l, false)
	fe.w.ln("%s%s;", m, lv.set(val))
}

// lvalue describes an assignable location.
type lvalue struct {
	get string
	set func(rhs string) string
}

// aggregateSet assigns an aggregate value in place so pointers to the
// destination observe the change.
func (fe *funcEmitter) aggregateSet(dst string, t types.Type, rhs string) string {
	switch t.Underlying().(type) {
	case *types.Struct:
		if n, ok := types.Unalias(t).(*types.Named); ok && n.TypeArgs().Len() > 0 {
			return fmt.Sprintf("%s.$set(%s, %s)", dst, rhs, fe.desc(t))
		}
		return fmt.Sprintf("%s.$set(%s)", dst, rhs)
	}
	return fmt.Sprintf("$rt.assign(%s, %s, %s)", fe.desc(t), dst, rhs)
}

func (fe *funcEmitter) simpleLvalue(ref string, t types.Type) lvalue {
	if _, isTP := types.Unalias(t).(*types.TypeParam); !isTP && isAggregate(t) {
		return lvalue{get: ref, set: func(rhs string) string { return fe.aggregateSet(ref, t, rhs) }}
	}
	return lvalue{get: ref, set: func(rhs string) string { return ref + " = " + rhs }}
}

func (fe *funcEmitter) lvalue(e ast.Expr, prepare bool) lvalue {
	// With prepare, operands are evaluated now into temporaries, even plain
	// references: in `i, a[i] = 1, 2` the index is the old i.
	stab := func(s string) string {
		if prepare && !jsLiteral.MatchString(stripMarks(s)) {
			return fe.forceTmp(s)
		}
		return s
	}
	t := fe.info.TypeOf(e)
	switch x := unparen(e).(type) {
	case *ast.Ident:
		if x.Name == "_" {
			return lvalue{get: "undefined", set: func(rhs string) string { return rhs }}
		}
		v := fe.info.Uses[x].(*types.Var)
		return fe.simpleLvalue(fe.varRef(v), t)
	case *ast.SelectorExpr:
		if _, ok := fe.info.Selections[x]; !ok {
			v := fe.info.Uses[x.Sel].(*types.Var) // package-qualified variable
			return fe.simpleLvalue(fe.varRef(v), t)
		}
		obj, prop := fe.fieldBase(x)
		return fe.simpleLvalue(stab(obj)+"."+prop, t)
	case *ast.IndexExpr:
		xt := fe.info.TypeOf(x.X)
		switch u := under(xt).(type) {
		case *types.Map:
			mp := stab(fe.expr(x.X))
			k := stab(fe.valueOf(x.Index, u.Key()))
			return lvalue{
				get: fmt.Sprintf("$rt.mapGet(%s, %s, %s)", mp, k, fe.zeroFn(u.Elem())),
				set: func(rhs string) string { return fmt.Sprintf("%s$rt.mapSet(%s, %s, %s)", fe.mark(x), mp, k, rhs) },
			}
		case *types.Slice:
			s := stab(fe.expr(x.X))
			i := stab(fe.expr(x.Index))
			get := fmt.Sprintf("%s$rt.index(%s, %s)", fe.mark(x), s, i)
			if isAggregate(u.Elem()) {
				return fe.simpleLvalue(get, t)
			}
			return lvalue{get: get, set: func(rhs string) string { return fmt.Sprintf("%s$rt.setIndex(%s, %s, %s)", fe.mark(x), s, i, rhs) }}
		default:
			a := stab(fe.expr(x.X))
			i := stab(fe.arrayIndex(x))
			return fe.simpleLvalue(a+"["+i+"]", t)
		}
	case *ast.StarExpr:
		p := stab(fe.expr(x.X))
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP {
			d := fe.desc(t)
			return lvalue{get: fmt.Sprintf("$rt.load(%s, %s)", d, p), set: func(rhs string) string { return fmt.Sprintf("$rt.store(%s, %s, %s)", d, p, rhs) }}
		}
		if isAggregate(t) {
			return fe.simpleLvalue(p, t)
		}
		return fe.simpleLvalue(p+".v", t)
	}
	fe.errorf(e.Pos(), "unsupported assignment target %T", e)
	return lvalue{get: "undefined", set: func(rhs string) string { return rhs }}
}

func (fe *funcEmitter) ifStmt(s *ast.IfStmt, label string) {
	w := fe.w
	if s.Init != nil {
		w.ln("%s{", labelPrefix(label))
		w.indent++
		fe.stmt(s.Init, "")
	} else if label != "" {
		w.ln("%s: {", label)
		w.indent++
	}
	w.ln("%sif (%s) {", fe.mark(s), fe.expr(s.Cond))
	fe.block(s.Body.List)
	switch e := s.Else.(type) {
	case nil:
		w.ln("}")
	case *ast.BlockStmt:
		w.ln("} else {")
		fe.block(e.List)
		w.ln("}")
	case *ast.IfStmt:
		w.ln("} else {")
		w.indent++
		fe.ifStmt(e, "")
		w.indent--
		w.ln("}")
	}
	if s.Init != nil || label != "" {
		w.indent--
		w.ln("}")
	}
}

// stmtExpr lowers a simple statement to a JS expression (for loop posts).
func (fe *funcEmitter) stmtExpr(s ast.Stmt) string {
	w := newRawWriter(fe.pe.tab)
	c := *fe
	c.w = w
	c.stmt(s, "")
	body := strings.TrimSpace(w.String())
	if strings.Count(body, "\n") == 0 && strings.HasSuffix(body, ";") && !strings.HasPrefix(stripMarks(body), "const ") && !strings.HasPrefix(stripMarks(body), "let ") {
		return strings.TrimSuffix(body, ";")
	}
	if fe.async {
		return "await (async () => { " + body + " })()"
	}
	return "(() => { " + body + " })()"
}

func (fe *funcEmitter) forStmt(s *ast.ForStmt, label string) {
	w := fe.w
	cond := ""
	post := ""
	perIter := fe.goVersionAtLeast("go1.22")
	// Go 1.22+: variables declared by the init statement are per-iteration,
	// which JS `for (let ...)` provides natively. A boxed loop variable needs
	// a fresh cell per iteration (copied before the post statement).
	if as, ok := s.Init.(*ast.AssignStmt); ok && as.Tok == token.DEFINE && perIter && len(as.Lhs) == len(as.Rhs) {
		var decls, renew []string
		for i, l := range as.Lhs {
			v, _ := fe.info.Defs[l.(*ast.Ident)].(*types.Var)
			if v == nil {
				decls = nil
				break
			}
			val := fe.valueOf(as.Rhs[i], v.Type())
			n := fe.declare(v)
			if fe.boxed(v) {
				decls = append(decls, fmt.Sprintf("%s = $rt.cell(%s)", n, val))
				renew = append(renew, fmt.Sprintf("%s = $rt.cell(%s.v)", n, n))
			} else {
				decls = append(decls, fmt.Sprintf("%s: %s = %s", n, fe.ts(v.Type()), val))
			}
		}
		if decls != nil {
			if s.Cond != nil {
				cond = fe.expr(s.Cond)
			}
			posts := renew
			if s.Post != nil {
				posts = append(posts, fe.stmtExpr(s.Post))
			}
			post = strings.Join(posts, ", ")
			w.ln("%s%sfor (let %s; %s; %s) {", fe.mark(s), labelPrefix(label), strings.Join(decls, ", "), cond, post)
			fe.block(s.Body.List)
			w.ln("}")
			return
		}
	}
	if s.Init != nil {
		w.ln("{")
		w.indent++
		fe.stmt(s.Init, "")
	}
	if s.Cond != nil {
		cond = fe.expr(s.Cond)
	}
	if s.Post != nil {
		post = fe.stmtExpr(s.Post)
	}
	w.ln("%s%sfor (; %s; %s) {", fe.mark(s), labelPrefix(label), cond, post)
	fe.block(s.Body.List)
	w.ln("}")
	if s.Init != nil {
		w.indent--
		w.ln("}")
	}
}

// rangeVars emits the per-iteration bindings of a range loop's key and value.
func (fe *funcEmitter) rangeVars(s *ast.RangeStmt, key, val string, keyT, valT types.Type) {
	bind := func(e ast.Expr, src string, srcT types.Type) {
		if e == nil {
			return
		}
		if id, ok := e.(*ast.Ident); ok && id.Name == "_" {
			return
		}
		if s.Tok == token.DEFINE {
			v := fe.info.Defs[e.(*ast.Ident)].(*types.Var)
			fe.defineVar("", v, fe.convert(src, srcT, v.Type()))
			return
		}
		lv := fe.lvalue(e, false)
		fe.w.ln("%s;", lv.set(fe.convert(src, srcT, fe.info.TypeOf(e))))
	}
	bind(s.Key, key, keyT)
	if val != "" {
		bind(s.Value, val, valT)
	}
}

func (fe *funcEmitter) rangeStmt(s *ast.RangeStmt, label string) {
	w := fe.w
	m := fe.mark(s)
	lp := labelPrefix(label)
	xt := fe.info.TypeOf(s.X)
	hasVal := s.Value != nil && !isBlank(s.Value)
	if !fe.goVersionAtLeast("go1.22") && s.Tok == token.DEFINE {
		fe.errorf(s.Pos(), "range loops in files with go < 1.22 (shared loop variables) are not supported yet")
	}
	ut := under(xt)
	if p, ok := ut.(*types.Pointer); ok {
		ut = p.Elem().Underlying() // *array
	}
	switch u := ut.(type) {
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			str, i, r, wd := fe.tmp(), fe.tmp(), fe.tmp(), fe.tmp()
			w.ln("%s%sfor (let %s = %s, %s = 0, %s = 0, %s = 0; %s < %s.length; %s += %s) {", m, lp, str, fe.expr(s.X), i, r, wd, i, str, i, wd)
			w.indent++
			w.ln("[%s, %s] = $rt.decodeRune(%s, %s);", r, wd, str, i)
			fe.rangeVars(s, i, r, types.Typ[types.Int], types.Typ[types.Int32])
			fe.stmts(s.Body.List)
			w.indent--
			w.ln("}")
			return
		}
		// range over integer (Go 1.22)
		i, n := fe.tmp(), fe.tmp()
		w.ln("%s%sfor (let %s = 0, %s = %s; %s < %s; %s++) {", m, lp, i, n, fe.expr(s.X), i, n, i)
		w.indent++
		fe.rangeVars(s, i, "", xt, nil)
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Slice:
		sl, i, n := fe.tmp(), fe.tmp(), fe.tmp()
		w.ln("%s%sfor (let %s = %s, %s = 0, %s = $rt.len(%s); %s < %s; %s++) {", m, lp, sl, fe.expr(s.X), i, n, sl, i, n, i)
		w.indent++
		val := ""
		if hasVal {
			val = fe.pe.copyExpr(fmt.Sprintf("%s!.$array[%s!.$offset + %s]", sl, sl, i), u.Elem(), fe.tp)
		}
		fe.rangeVars(s, i, val, types.Typ[types.Int], u.Elem())
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Array:
		arr, i := fe.tmp(), fe.tmp()
		src := fe.expr(s.X)
		if _, isPtr := xt.Underlying().(*types.Pointer); !isPtr && hasVal {
			src = fe.pe.copyExpr(src, xt, fe.tp) // the range expression is a copy
		}
		w.ln("%s%sfor (let %s = %s, %s = 0; %s < %d; %s++) {", m, lp, arr, src, i, i, u.Len(), i)
		w.indent++
		val := ""
		if hasVal {
			val = fe.pe.copyExpr(fmt.Sprintf("%s[%s]", arr, i), u.Elem(), fe.tp)
		}
		fe.rangeVars(s, i, val, types.Typ[types.Int], u.Elem())
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Map:
		k, v := fe.tmp(), fe.tmp()
		w.ln("%s%sfor (const [%s, %s] of $rt.mapRange(%s)) {", m, lp, k, v, fe.expr(s.X))
		w.indent++
		val := ""
		if hasVal {
			val = fe.pe.copyExpr(v, u.Elem(), fe.tp)
		}
		fe.rangeVars(s, fe.pe.copyExpr(k, u.Key(), fe.tp), val, u.Key(), u.Elem())
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Chan:
		ch, r := fe.tmp(), fe.tmp()
		w.ln("%sconst %s = %s;", m, ch, fe.expr(s.X))
		w.ln("%sfor (;;) {", lp)
		w.indent++
		w.ln("const %s = await $rt.recv(%s);", r, ch)
		w.ln("if (!%s[1]) break;", r)
		fe.rangeVars(s, r+"[0]", "", u.Elem(), nil)
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Signature:
		fe.rangeFunc(s, label, u)
	default:
		fe.errorf(s.Pos(), "unsupported range over %s", xt)
	}
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// rangeFunc lowers range-over-func (Go 1.23 iterators) to a call with a
// yield callback. Supported: break/continue of this loop and return; the
// body must not block.
func (fe *funcEmitter) rangeFunc(s *ast.RangeStmt, label string, sig *types.Signature) {
	w := fe.w
	bad := false
	ast.Inspect(s.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BranchStmt:
			if n.Label != nil || n.Tok == token.GOTO || n.Tok == token.FALLTHROUGH {
				bad = true
			}
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			// unlabeled break/continue inside these target them, fine
			return true
		case *ast.SendStmt, *ast.DeferStmt:
			bad = true
		case *ast.UnaryExpr:
			if n.Op == token.ARROW {
				bad = true
			}
		}
		return true
	})
	if bad {
		fe.errorf(s.Pos(), "range-over-func with labeled branches, goto, defer or blocking operations in the body is not supported yet")
		return
	}
	yield := sig.Params().At(0).Type().Underlying().(*types.Signature)
	var params []string
	var ptypes []types.Type
	for i := 0; i < yield.Params().Len(); i++ {
		params = append(params, fe.tmp())
		ptypes = append(ptypes, yield.Params().At(i).Type())
	}
	ret, retv := fe.tmp(), fe.tmp()
	w.ln("%slet %s = false, %s: any;", fe.mark(s), ret, retv)
	call := fmt.Sprintf("%s((%s) => {", fe.expr(s.X), strings.Join(params, ", "))
	if fe.pe.prog.RangeBlocks(fe.info, s) {
		call = "await " + call
	}
	w.ln("%s", call)
	w.indent++
	k, v := "", ""
	var kt, vt types.Type
	if len(params) > 0 {
		k, kt = params[0], ptypes[0]
	}
	if len(params) > 1 {
		v, vt = params[1], ptypes[1]
	}
	if k != "" {
		fe.rangeVars(s, k, v, kt, vt)
	}
	// Lower the body with break/continue/return rewritten for the callback.
	c := *fe
	c.w = newRawWriter(fe.pe.tab)
	c.w.indent = w.indent
	body := &rangeFuncBody{fe: &c, ret: ret, retv: retv}
	body.stmts(s.Body.List)
	w.write(c.w.String())
	w.ln("return true;")
	w.indent--
	w.ln("});")
	if fe.sig != nil {
		if fe.hasDefer {
			w.ln("if (%s) { %s; break $body; }", ret, fe.assignResults(retv))
		} else if fe.sig.Results().Len() > 0 {
			w.ln("if (%s) return %s;", ret, retv)
		} else {
			w.ln("if (%s) return;", ret)
		}
	}
}

func (fe *funcEmitter) assignResults(tuple string) string {
	switch len(fe.results) {
	case 0:
		return "void 0"
	case 1:
		return fe.results[0] + " = " + tuple
	}
	var parts []string
	for i, r := range fe.results {
		parts = append(parts, fmt.Sprintf("%s = %s[%d]", r, tuple, i))
	}
	return strings.Join(parts, ", ")
}

type rangeFuncBody struct {
	fe        *funcEmitter
	ret, retv string
}

// stmts emits the loop body; branch statements that target the range loop
// itself and returns are rewritten. Nested loops/switches are emitted
// normally since their own unlabeled break/continue stay local.
func (b *rangeFuncBody) stmts(list []ast.Stmt) {
	for _, s := range list {
		b.stmt(s)
	}
}

func (b *rangeFuncBody) stmt(s ast.Stmt) {
	fe := b.fe
	w := fe.w
	switch s := s.(type) {
	case *ast.BranchStmt:
		if s.Tok == token.BREAK {
			w.ln("%sreturn false;", fe.mark(s))
		} else {
			w.ln("%sreturn true;", fe.mark(s))
		}
	case *ast.ReturnStmt:
		vals := fe.returnValues(s)
		v := "undefined"
		switch len(vals) {
		case 0:
			if r := fe.resultsExpr(); r != "" {
				v = r
			}
		case 1:
			v = vals[0]
		default:
			v = "[" + strings.Join(vals, ", ") + "]"
		}
		w.ln("%s%s = true; %s = %s; return false;", fe.mark(s), b.ret, b.retv, v)
	case *ast.BlockStmt:
		w.ln("{")
		w.indent++
		b.stmts(s.List)
		w.indent--
		w.ln("}")
	case *ast.IfStmt:
		if s.Init != nil {
			w.ln("{")
			w.indent++
			fe.stmt(s.Init, "")
		}
		w.ln("%sif (%s) {", fe.mark(s), fe.expr(s.Cond))
		w.indent++
		b.stmts(s.Body.List)
		w.indent--
		if s.Else != nil {
			w.ln("} else {")
			w.indent++
			b.stmt(s.Else)
			w.indent--
		}
		w.ln("}")
		if s.Init != nil {
			w.indent--
			w.ln("}")
		}
	default:
		if containsBranchOrReturn(s) {
			fe.errorf(s.Pos(), "this statement form inside a range-over-func body is not supported yet")
			return
		}
		fe.stmt(s, "")
	}
}

func containsBranchOrReturn(s ast.Stmt) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		case *ast.ReturnStmt:
			found = true
		case *ast.BranchStmt:
			found = true
		}
		return !found
	})
	if found {
		// switch statements may contain unlabeled breaks targeting themselves;
		// those are fine.
		if _, ok := s.(*ast.SwitchStmt); ok {
			return containsReturn(s)
		}
	}
	return found
}

func containsReturn(s ast.Node) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if _, ok := n.(*ast.ReturnStmt); ok {
			found = true
		}
		return !found
	})
	return found
}

// switchStmt lowers a Go switch to case selection followed by a JS switch on
// the selected case index. Go case expressions are evaluated lazily in source
// order, and `fallthrough` maps to JS fallthrough.
func (fe *funcEmitter) switchStmt(s *ast.SwitchStmt, label string) {
	w := fe.w
	w.ln("{")
	w.indent++
	if s.Init != nil {
		fe.stmt(s.Init, "")
	}
	var tag string
	var tagT types.Type
	if s.Tag != nil {
		tagT = fe.info.TypeOf(s.Tag)
		tag = fe.forceTmp(fe.valueOf(s.Tag, tagT))
	}
	c := fe.tmp()
	w.ln("let %s = -1;", c)
	clauses := s.Body.List
	def := -1
	first := true
	for i, cl := range clauses {
		cc := cl.(*ast.CaseClause)
		if cc.List == nil {
			def = i
			continue
		}
		var conds []string
		for _, e := range cc.List {
			if s.Tag == nil {
				conds = append(conds, "("+fe.expr(e)+")")
			} else {
				conds = append(conds, fe.equal(tag, tagT, e))
			}
		}
		kw := "else if"
		if first {
			kw = "if"
			first = false
		}
		w.ln("%s%s (%s) %s = %d;", fe.mark(cc), kw, strings.Join(conds, " || "), c, i)
	}
	if def >= 0 {
		if first {
			w.ln("%s = %d;", c, def)
		} else {
			w.ln("else %s = %d;", c, def)
		}
	}
	fe.caseBodies(s.Pos(), c, label, clauses, nil)
	w.indent--
	w.ln("}")
}

func (fe *funcEmitter) caseBodies(pos token.Pos, c, label string, clauses []ast.Stmt, prologue func(i int)) {
	w := fe.w
	w.ln("%s%sswitch (%s) {", fe.pe.tab.mark(pos), labelPrefix(label), c)
	w.indent++
	for i, cl := range clauses {
		var body []ast.Stmt
		switch cl := cl.(type) {
		case *ast.CaseClause:
			body = cl.Body
		case *ast.CommClause:
			body = cl.Body
		}
		idx := i
		if cc, ok := cl.(*ast.CommClause); ok && cc.Comm == nil {
			idx = -1
		}
		w.ln("case %d: {", idx)
		w.indent++
		if prologue != nil {
			prologue(i)
		}
		fe.stmts(body)
		w.indent--
		fall := false
		if len(body) > 0 {
			if b, ok := body[len(body)-1].(*ast.BranchStmt); ok && b.Tok == token.FALLTHROUGH {
				fall = true
			}
		}
		if fall {
			w.ln("}")
		} else {
			w.ln("} break;")
		}
	}
	w.indent--
	w.ln("}")
}

func (fe *funcEmitter) typeSwitchStmt(s *ast.TypeSwitchStmt, label string) {
	w := fe.w
	w.ln("{")
	w.indent++
	if s.Init != nil {
		fe.stmt(s.Init, "")
	}
	var x ast.Expr
	switch a := s.Assign.(type) {
	case *ast.AssignStmt:
		x = a.Rhs[0].(*ast.TypeAssertExpr).X
	case *ast.ExprStmt:
		x = a.X.(*ast.TypeAssertExpr).X
	}
	xv := fe.forceTmp(fe.expr(x))
	c := fe.tmp()
	w.ln("let %s = -1;", c)
	clauses := s.Body.List
	def := -1
	first := true
	for i, cl := range clauses {
		cc := cl.(*ast.CaseClause)
		if cc.List == nil {
			def = i
			continue
		}
		var conds []string
		for _, e := range cc.List {
			t := fe.info.TypeOf(e)
			if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
				conds = append(conds, xv+" === null")
			} else {
				conds = append(conds, fmt.Sprintf("$rt.typeIs(%s, %s)", xv, fe.desc(t)))
			}
		}
		kw := "else if"
		if first {
			kw = "if"
			first = false
		}
		w.ln("%s%s (%s) %s = %d;", fe.mark(cc), kw, strings.Join(conds, " || "), c, i)
	}
	if def >= 0 {
		if first {
			w.ln("%s = %d;", c, def)
		} else {
			w.ln("else %s = %d;", c, def)
		}
	}
	fe.caseBodies(s.Pos(), c, label, clauses, func(i int) {
		cc := clauses[i].(*ast.CaseClause)
		obj, ok := fe.info.Implicits[cc].(*types.Var)
		if !ok {
			return
		}
		val := xv
		if len(cc.List) == 1 {
			t := fe.info.TypeOf(cc.List[0])
			if !isIface(t) {
				if b, ok := t.(*types.Basic); !ok || b.Kind() != types.UntypedNil {
					val = fe.pe.copyExpr(xv+"!.v", t, fe.tp)
				}
			}
		}
		fe.defineVar("", obj, val)
	})
	w.indent--
	w.ln("}")
}

func (fe *funcEmitter) selectStmt(s *ast.SelectStmt, label string) {
	w := fe.w
	var cases []string
	hasDefault := false
	for _, cl := range s.Body.List {
		cc := cl.(*ast.CommClause)
		switch comm := cc.Comm.(type) {
		case nil:
			hasDefault = true
			cases = append(cases, "")
		case *ast.SendStmt:
			elem := under(fe.info.TypeOf(comm.Chan)).(*types.Chan).Elem()
			cases = append(cases, fmt.Sprintf("[%s, true, %s]", fe.expr(comm.Chan), fe.valueOf(comm.Value, elem)))
		default:
			var recv ast.Expr
			switch c := comm.(type) {
			case *ast.ExprStmt:
				recv = c.X
			case *ast.AssignStmt:
				recv = c.Rhs[0]
			}
			cases = append(cases, fmt.Sprintf("[%s, false, undefined]", fe.expr(unparen(recv).(*ast.UnaryExpr).X)))
		}
	}
	var real []string
	for _, c := range cases {
		if c != "" {
			real = append(real, c)
		}
	}
	// Map JS case indices (non-default cases) back to clauses.
	w.ln("{")
	w.indent++
	sel := fe.tmp()
	aw := "await "
	if hasDefault {
		aw = ""
	}
	w.ln("%sconst %s = %s$rt.select([%s], %v);", fe.mark(s), sel, aw, strings.Join(real, ", "), hasDefault)
	w.ln("%sswitch (%s[0]) {", labelPrefix(label), sel)
	w.indent++
	j := 0
	for _, cl := range s.Body.List {
		cc := cl.(*ast.CommClause)
		idx := -1
		if cc.Comm != nil {
			idx = j
			j++
		}
		w.ln("case %d: {", idx)
		w.indent++
		if as, ok := cc.Comm.(*ast.AssignStmt); ok {
			ch := unparen(as.Rhs[0]).(*ast.UnaryExpr).X
			elem := under(fe.info.TypeOf(ch)).(*types.Chan).Elem()
			vals := []string{sel + "[1]", sel + "[2]"}
			vts := []types.Type{elem, types.Typ[types.Bool]}
			for i, l := range as.Lhs {
				fe.assignOne(as.Tok, l, fe.convert(vals[i], vts[i], fe.lhsType(l, vts[i])), "")
			}
		}
		fe.stmts(cc.Body)
		w.indent--
		w.ln("} break;")
	}
	w.indent--
	w.ln("}")
	w.indent--
	w.ln("}")
}

// returnValues lowers the operands of a return statement.
func (fe *funcEmitter) returnValues(s *ast.ReturnStmt) []string {
	res := fe.sig.Results()
	if len(s.Results) == 0 {
		return nil
	}
	if len(s.Results) == 1 && res.Len() > 1 {
		t := fe.forceTmp(fe.expr(s.Results[0]))
		tt := fe.info.TypeOf(s.Results[0])
		var vals []string
		for i := 0; i < res.Len(); i++ {
			vals = append(vals, fe.convert(fmt.Sprintf("%s[%d]", t, i), tupleAt(tt, i), res.At(i).Type()))
		}
		return vals
	}
	var vals []string
	for i, r := range s.Results {
		vals = append(vals, fe.valueOf(r, res.At(i).Type()))
	}
	return vals
}

func (fe *funcEmitter) returnStmt(s *ast.ReturnStmt) {
	w := fe.w
	m := fe.mark(s)
	vals := fe.returnValues(s)
	if fe.hasDefer {
		if len(vals) == 1 {
			w.ln("%s%s = %s;", m, fe.results[0], vals[0])
		} else if len(vals) > 1 {
			var tmps []string
			for _, v := range vals {
				tmps = append(tmps, fe.forceTmp(v))
			}
			for i, t := range tmps {
				w.ln("%s = %s;", fe.results[i], t)
			}
		}
		w.ln("%sbreak $body;", m)
		return
	}
	switch len(vals) {
	case 0:
		if r := fe.resultsExpr(); r != "" {
			w.ln("%sreturn %s;", m, r)
		} else {
			w.ln("%sreturn;", m)
		}
	case 1:
		w.ln("%sreturn %s;", m, vals[0])
	default:
		w.ln("%sreturn [%s];", m, strings.Join(vals, ", "))
	}
}

// deferredCall evaluates the function value and arguments of a go/defer
// call now (as Go requires) and returns a closure performing the call later.
func (fe *funcEmitter) deferredCall(call *ast.CallExpr) string {
	if lit, ok := unparen(call.Fun).(*ast.FuncLit); ok && len(call.Args) == 0 {
		return fe.funcLit(lit)
	}
	var overridden []ast.Expr
	set := func(e ast.Expr, v string) {
		fe.override[e] = fe.forceTmp(v)
		overridden = append(overridden, e)
	}
	fun := unparen(call.Fun)
	isConv := false
	if tv, ok := fe.info.Types[fun]; ok && tv.IsType() {
		isConv = true
	}
	if !isConv {
		switch f := fun.(type) {
		case *ast.Ident:
			if _, ok := fe.info.Uses[f].(*types.Var); ok {
				set(f, fe.expr(f))
			}
		case *ast.SelectorExpr:
			if sel, ok := fe.info.Selections[f]; ok {
				switch sel.Kind() {
				case types.MethodVal:
					fn := sel.Obj().(*types.Func)
					_, ptrRecv := fn.Signature().Recv().Type().(*types.Pointer)
					xt := fe.info.TypeOf(f.X)
					_, xPtr := xt.Underlying().(*types.Pointer)
					switch {
					case ptrRecv && !xPtr && !isAggregate(xt):
						// address of a boxed variable is stable; evaluate lazily
					case !xPtr && isAggregate(xt) && !ptrRecv:
						set(f.X, fe.pe.copyExpr(fe.expr(f.X), xt, fe.tp))
					default:
						set(f.X, fe.expr(f.X))
					}
				case types.FieldVal:
					set(f, fe.expr(f))
				}
			}
		case *ast.FuncLit:
			set(f, fe.funcLit(f))
		default:
			if _, ok := under(fe.info.TypeOf(fun)).(*types.Signature); ok {
				if _, isInst := fe.info.Instances[identOf(fun)]; !isInst {
					set(fun, fe.expr(fun))
				}
			}
		}
	}
	for _, a := range call.Args {
		t := fe.info.TypeOf(a)
		if _, ok := t.(*types.Tuple); ok {
			set(a, fe.expr(a))
			continue
		}
		set(a, fe.pe.copyExpr(fe.expr(a), t, fe.tp))
	}
	body := fe.expr(call)
	for _, e := range overridden {
		delete(fe.override, e)
	}
	if fe.pe.prog.CallBlocks(fe.info, call) || strings.Contains(body, "await ") {
		return "async () => { " + body + "; }"
	}
	return "() => { " + body + "; }"
}
