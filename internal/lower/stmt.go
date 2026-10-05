package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"strings"
)

// stmts lowers a statement list. A forward goto to a label of this list
// becomes a break out of a JS labelled block that ends right before the
// labelled statement: Go forbids jumping into blocks and over variable
// declarations, so the block only wraps statements that declare nothing
// visible at the label.
func (fe *funcEmitter) stmts(list []ast.Stmt) {
	if fe.backwardGoto(list) {
		fe.gotoMachine(list)
		return
	}
	type span struct {
		start, end int // wrap list[start:end]; list[end] is the label
		name       string
	}
	var spans []*span
	for k, s := range list {
		ls, ok := s.(*ast.LabeledStmt)
		if !ok {
			continue
		}
		label := fe.info.Defs[ls.Label]
		var gotos []*ast.BranchStmt
		start := -1
		for i := 0; i < k; i++ {
			ast.Inspect(list[i], func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					return false
				case *ast.BranchStmt:
					if n.Tok == token.GOTO && fe.info.Uses[n.Label] == label {
						gotos = append(gotos, n)
						if start < 0 {
							start = i
						}
					}
				}
				return true
			})
		}
		if start < 0 {
			continue
		}
		if fe.gotos == nil {
			fe.gotos = map[*ast.BranchStmt]bool{}
		}
		for _, g := range gotos {
			fe.gotos[g] = true
		}
		sp := &span{start, k, "G$" + ls.Label.Name}
		// Spans must nest: one that starts inside an earlier label's span
		// is extended to that span's start.
		for _, o := range spans {
			if sp.start > o.start && sp.start < o.end {
				sp.start = o.start
			}
		}
		spans = append(spans, sp)
	}
	var open []*span
	for i, s := range list {
		for len(open) > 0 && open[len(open)-1].end == i {
			fe.w.indent--
			fe.w.ln("}")
			open = open[:len(open)-1]
		}
		// Open the spans starting here, outermost (latest label) first.
		for j := len(spans) - 1; j >= 0; j-- {
			if sp := spans[j]; sp.start == i {
				fe.w.ln("%s: {", sp.name)
				fe.w.indent++
				open = append(open, sp)
			}
		}
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
		if u, ok := unparen(s.X).(*ast.UnaryExpr); ok && u.Op == token.ARROW {
			// A receive statement: the value is discarded.
			if fe.syncOnly {
				w.ln("%s%s$rt.recvNow(%s);", m, fe.mark(u), fe.expr(u.X))
			} else {
				w.ln("%s%s%s;", m, fe.mark(u), fe.awaitOp("$rt.recv("+fe.expr(u.X)+")"))
			}
			return
		}
		if fe.raceNoop(s.X) {
			return
		}
		w.ln("%s%s;", m, fe.expr(s.X))
	case *ast.DeclStmt:
		if fe.splitStmt(s, m) {
			return
		}
		fe.declStmt(s)
	case *ast.AssignStmt:
		if fe.splitStmt(s, m) {
			return
		}
		fe.assign(s)
	case *ast.IncDecStmt:
		if fe.splitStmt(s, m) {
			return
		}
		lv := fe.lvalue(s.X, true)
		t := fe.info.TypeOf(s.X)
		op := token.ADD
		if s.Tok == token.DEC {
			op = token.SUB
		}
		one := "1"
		switch {
		case isTypeParam(t):
			one = "$rt.constT(" + fe.desc(t) + ", 1)"
		case isBig(t):
			one = "1n"
		case isComplex(t):
			one = "$rt.complex(1, 0)"
		}
		w.ln("%s%s;", m, lv.set(fe.arith(op, lv.get, one, t)))
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
			// Only goto can target this label (lowered by stmts), and a
			// JS block would scope a declaration in s.Stmt.
			fe.stmt(s.Stmt, "")
		}
	case *ast.IfStmt:
		fe.ifStmt(s, label)
	case *ast.ForStmt:
		label = fe.pushBreakable(s, label, true)
		fe.forStmt(s, label)
		fe.popBreakable()
	case *ast.RangeStmt:
		label = fe.pushBreakable(s, label, true)
		fe.rangeStmt(s, label)
		fe.popBreakable()
	case *ast.SwitchStmt:
		label = fe.pushBreakable(s, label, false)
		fe.switchStmt(s, label)
		fe.popBreakable()
	case *ast.TypeSwitchStmt:
		label = fe.pushBreakable(s, label, false)
		fe.typeSwitchStmt(s, label)
		fe.popBreakable()
	case *ast.SelectStmt:
		label = fe.pushBreakable(s, label, false)
		fe.selectStmt(s, label)
		fe.popBreakable()
	case *ast.ReturnStmt:
		if fe.rangeFn != nil {
			fe.rangeFuncReturn(s)
			return
		}
		fe.returnStmt(s)
	case *ast.BranchStmt:
		if rf := fe.rangeFn; rf != nil {
			if rf.branches[s] {
				w.ln("%s%s", m, rf.next(s.Tok == token.CONTINUE))
				return
			}
			if k := rf.exits[s]; k != 0 {
				w.ln("%s%s = %d; %s", m, rf.ret, k, rf.next(false))
				return
			}
		}
		switch s.Tok {
		case token.BREAK, token.CONTINUE:
			kw := "break"
			if s.Tok == token.CONTINUE {
				kw = "continue"
			}
			if s.Label != nil {
				w.ln("%s%s %s;", m, kw, fe.labelName(s.Label))
			} else if l := fe.branchLabel(s.Tok == token.CONTINUE); l != "" {
				w.ln("%s%s %s;", m, kw, l)
			} else {
				w.ln("%s%s;", m, kw)
			}
		case token.FALLTHROUGH:
			// handled by switchStmt: the JS case is emitted without break
		case token.GOTO:
			if t, ok := fe.gotoTargets[fe.info.Uses[s.Label]]; ok {
				w.ln("%s%s = %d; continue %s;", m, t.state, t.n, t.loop)
				return
			}
			if !fe.gotos[s] {
				fe.errorf(s.Pos(), "goto %s is not supported here", s.Label.Name)
			}
			w.ln("%sbreak G$%s;", m, s.Label.Name)
		}
	case *ast.GoStmt:
		closure := fe.deferredCall(s.Call)
		w.ln("%s$rt.go(%s);", m, closure)
	case *ast.DeferStmt:
		closure := fe.deferredCall(s.Call)
		if tok := fe.deferredTok(s.Call); tok != "" {
			w.ln("%s$d.defer(%s, %s);", m, closure, jsString(tok))
		} else {
			w.ln("%s$d.defer(%s);", m, closure)
		}
	case *ast.SendStmt:
		ch := fe.expr(s.Chan)
		elem := under(fe.info.TypeOf(s.Chan)).(*types.Chan).Elem()
		if fe.syncOnly {
			w.ln("%s$rt.sendNow(%s, %s);", m, ch, fe.valueOf(s.Value, elem))
		} else {
			w.ln("%s%s;", m, fe.awaitOp(fmt.Sprintf("$rt.send(%s, %s)", ch, fe.valueOf(s.Value, elem))))
		}
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
		default: // tuple: f() or a comma-ok form (v, ok = m[k], <-ch, x.(T))
			e, tt, ok := fe.commaOk(vs.Values[0])
			if !ok {
				e, tt = fe.expr(vs.Values[0]), fe.info.TypeOf(vs.Values[0])
			}
			t := fe.forceTmp(e)
			for i, v := range vars {
				fe.defineVar(m, v, fe.convert(fmt.Sprintf("%s[%d]", t, i), tupleAt(tt, i), v.Type()))
			}
		}
	}
}

// defineVar declares a new local variable with an initial value.
func (fe *funcEmitter) defineVar(m string, v *types.Var, init string) {
	if v == nil || v.Name() == "_" {
		fe.discard(m, init)
		return
	}
	n := fe.declare(v)
	if fe.hoisted[v] {
		if fe.boxed(v) {
			init = "$rt.cell(" + init + ")"
		}
		fe.w.ln("%s%s = %s;", m, n, init)
		return
	}
	if fe.boxed(v) {
		fe.w.ln("%slet %s: $rt.Cell<%s> = $rt.cell(%s);", m, n, fe.ts(v.Type()), init)
		return
	}
	ts := fe.ts(v.Type())
	if fe.pe.strBytes[v] {
		ts = "string"
	}
	fe.w.ln("%slet %s: %s = %s;", m, n, ts, init)
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
			return fe.mark(x) + fe.recvExpr(fe.expr(x.X)),
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
			val = fe.shift(op, lv.get, fe.shiftCount(s.Rhs[0]), t)
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
			if isBlank(l) {
				continue
			}
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
		if !isBlank(t.e) {
			w.ln("%s;", t.lv.set(vals[i]))
		}
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
			fe.discard(m, val)
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

// tupleIndex matches a lowered read of one element of a tuple value.
var tupleIndex = regexp.MustCompile(`^(.*)\[\d+\]$`)

// discard evaluates a lowered value assigned to the blank identifier. Reads
// without effects (a variable, a literal, an element of a tuple held in a
// temporary) are dropped; for `_ = (call)[i]` only the call is kept.
func (fe *funcEmitter) discard(m, val string) {
	v := stripMarks(val)
	if g := tupleIndex.FindStringSubmatch(v); g != nil {
		if jsIdent.MatchString(g[1]) {
			return
		}
		if inner, ok := parenthesized(g[1]); ok {
			fe.w.ln("%s%s;", m, inner)
			return
		}
	}
	if jsIdent.MatchString(v) || jsLiteral.MatchString(v) {
		return
	}
	fe.w.ln("%s%s;", m, val)
}

// raceNoop reports whether x is a call of an internal/race function, which
// does nothing without the race detector, whose arguments have no effect:
// iter.Pull calls race.Acquire(unsafe.Pointer(&pull.racer)) for each value,
// and taking the address of a field allocates a pointer object.
func (fe *funcEmitter) raceNoop(x ast.Expr) bool {
	call, ok := unparen(x).(*ast.CallExpr)
	if !ok {
		return false
	}
	fn := calledFunc(fe.info, call)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "internal/race" {
		return false
	}
	for _, a := range call.Args {
		if !fe.pureArg(a) {
			return false
		}
	}
	return true
}

// pureArg reports whether evaluating e has no effect and cannot panic: a
// name, a literal, or the address of a variable or of a field of a struct
// variable, possibly converted.
func (fe *funcEmitter) pureArg(e ast.Expr) bool {
	switch e := unparen(e).(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.CallExpr:
		if tv, ok := fe.info.Types[e.Fun]; ok && tv.IsType() && len(e.Args) == 1 {
			return fe.pureArg(e.Args[0])
		}
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			x := unparen(e.X)
			for {
				sel, ok := x.(*ast.SelectorExpr)
				if !ok {
					break
				}
				if s, ok := fe.info.Selections[sel]; !ok || s.Kind() != types.FieldVal || s.Indirect() {
					return false // through a pointer, which may be nil
				}
				x = unparen(sel.X)
			}
			_, ok := x.(*ast.Ident)
			return ok
		}
	}
	return false
}

var jsIdent = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)

// parenthesized returns s without its enclosing parentheses if they match.
func parenthesized(s string) (string, bool) {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return "", false
	}
	depth := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return "", false
			}
		}
	}
	return s[1 : len(s)-1], true
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
		if isGenericType(t) {
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
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP && fe.boxed(v) {
			n := fe.nameOf(v)
			return lvalue{get: n + ".v", set: func(rhs string) string {
				return fmt.Sprintf("$rt.tpSet(%s, %s, %s)", fe.desc(t), n, rhs)
			}}
		}
		return fe.simpleLvalue(fe.varRef(v), t)
	case *ast.SelectorExpr:
		if _, ok := fe.info.Selections[x]; !ok {
			v := fe.info.Uses[x.Sel].(*types.Var) // package-qualified variable
			return fe.simpleLvalue(fe.varRef(v), t)
		}
		// p.f = v, ...: p is evaluated first, the nil check happens when
		// this assignment is carried out (nilChecked).
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
			if s, i, ok := fe.checkedIndex(x); ok {
				get := fe.mark(x) + sliceElem(s, i)
				if isAggregate(u.Elem()) {
					return fe.simpleLvalue(get, t)
				}
				return lvalue{get: fe.byteBoolLoad(x.X, get), set: func(rhs string) string { return get + " = " + fe.byteBoolStore(x.X, rhs) }}
			}
			s := stab(fe.expr(x.X))
			i := stab(fe.intNumber(x.Index))
			get := fe.byteBoolLoad(x.X, fe.mark(x)+sliceIndex(s, i))
			if isAggregate(u.Elem()) {
				return fe.simpleLvalue(get, t)
			}
			return lvalue{get: get, set: func(rhs string) string { return fe.mark(x) + setSliceIndex(s, i, fe.byteBoolStore(x.X, rhs)) }}
		case *types.Interface: // type parameter without a core type ([]E | [n]E)
			s := stab(fe.expr(x.X))
			i := stab(fe.intNumber(x.Index))
			return lvalue{
				get: fmt.Sprintf("%s$rt.indexAny(%s, %s)", fe.mark(x), s, i),
				set: func(rhs string) string { return fmt.Sprintf("%s$rt.setIndexAny(%s, %s, %s)", fe.mark(x), s, i, rhs) },
			}
		default:
			a := fe.expr(x.X)
			if _, isPtr := under(xt).(*types.Pointer); isPtr {
				a = nilChecked(a)
			}
			a = stab(a)
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
			return fe.simpleLvalue("$rt.deref("+p+")", t)
		}
		return fe.simpleLvalue(nilChecked(p)+".v", t)
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
	// Temporaries the statement hoisted are declared with the function's.
	fe.temps, fe.ir = c.temps, c.ir
	body := strings.TrimSpace(w.String())
	if strings.Count(body, "\n") == 0 && strings.HasSuffix(body, ";") && !strings.HasPrefix(stripMarks(body), "const ") && !strings.HasPrefix(stripMarks(body), "let ") {
		return strings.TrimSuffix(body, ";")
	}
	if fe.async {
		return fe.await("(async () => { " + body + " })()")
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
				decls = append(decls, fmt.Sprintf("%s: $rt.Cell<%s> = $rt.cell(%s)", n, fe.ts(v.Type()), val))
				renew = append(renew, fmt.Sprintf("%s = $rt.cell(%s)", n, fe.pe.copyExpr(n+".v", v.Type(), fe.tp)))
			} else {
				decls = append(decls, fmt.Sprintf("%s: %s = %s", n, fe.ts(v.Type()), val))
				if isAggregate(v.Type()) { // the next iteration gets its own copy
					renew = append(renew, fmt.Sprintf("%s = %s", n, fe.pe.copyExpr(n, v.Type(), fe.tp)))
				}
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
	use := func(e ast.Expr) bool { return e != nil && !isBlank(e) }
	if s.Tok == token.DEFINE {
		for _, b := range []struct {
			e    ast.Expr
			src  string
			srcT types.Type
		}{{s.Key, key, keyT}, {s.Value, val, valT}} {
			if use(b.e) && b.src != "" {
				v := fe.info.Defs[b.e.(*ast.Ident)].(*types.Var)
				if fields, ok := fe.scalarRangeVar(s, v); ok {
					fe.defineFields(v, b.src, fields)
					continue
				}
				val := fe.convert(b.src, b.srcT, v.Type())
				if fe.sharedRangeVars[v] {
					fe.w.ln("%s;", fe.simpleLvalue(fe.varRef(v), v.Type()).set(val))
					continue
				}
				fe.defineVar("", v, val)
			}
		}
		return
	}
	// for k, v = range x assigns like k, v = key, val: the operands of both
	// targets are evaluated before either is assigned (for i, a[i] = ...).
	both := use(s.Key) && use(s.Value) && val != ""
	var klv, vlv lvalue
	if use(s.Key) {
		klv = fe.lvalue(s.Key, both)
	}
	if use(s.Value) && val != "" {
		vlv = fe.lvalue(s.Value, both)
	}
	if use(s.Key) {
		fe.w.ln("%s;", klv.set(fe.convert(key, keyT, fe.info.TypeOf(s.Key))))
	}
	if use(s.Value) && val != "" {
		fe.w.ln("%s;", vlv.set(fe.convert(val, valT, fe.info.TypeOf(s.Value))))
	}
}

func (fe *funcEmitter) rangeStmt(s *ast.RangeStmt, label string) {
	if fe.goVersionAtLeast("go1.22") || s.Tok != token.DEFINE {
		fe.rangeLoop(s, label)
		return
	}
	// Before Go 1.22, the variables a range clause declares are shared by
	// all iterations: they are declared once, around the loop, and each
	// iteration assigns them.
	fe.w.ln("{")
	fe.w.indent++
	for _, e := range []ast.Expr{s.Key, s.Value} {
		if e == nil || isBlank(e) {
			continue
		}
		if v, ok := fe.info.Defs[e.(*ast.Ident)].(*types.Var); ok {
			fe.defineVar(fe.mark(s), v, fe.zero(v.Type()))
			if fe.sharedRangeVars == nil {
				fe.sharedRangeVars = map[*types.Var]bool{}
			}
			fe.sharedRangeVars[v] = true
		}
	}
	fe.rangeLoop(s, label)
	fe.w.indent--
	fe.w.ln("}")
}

func (fe *funcEmitter) rangeLoop(s *ast.RangeStmt, label string) {
	w := fe.w
	m := fe.mark(s)
	lp := labelPrefix(label)
	xt := fe.info.TypeOf(s.X)
	hasVal := s.Value != nil && !isBlank(s.Value)
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
		zero := "0"
		switch {
		case isTypeParam(xt):
			zero = "$rt.constT(" + fe.desc(xt) + ", 0)"
		case isBig(xt):
			zero = "0n"
		}
		w.ln("%s%sfor (let %s = %s, %s = %s; %s < %s; %s++) {", m, lp, i, zero, n, fe.expr(s.X), i, n, i)
		w.indent++
		fe.rangeVars(s, i, "", xt, nil)
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Slice:
		if str, ok := stringToBytesArg(fe.info, s.X); ok {
			// range []byte(s) ranges over the bytes of s: the body cannot
			// reach the slice the conversion would copy them into.
			st, i, n := fe.tmp(), fe.tmp(), fe.tmp()
			w.ln("%s%sfor (let %s = %s, %s = 0, %s = %s.length; %s < %s; %s++) {", m, lp, st, fe.expr(str), i, n, st, i, n, i)
			w.indent++
			val := ""
			if hasVal {
				val = st + ".charCodeAt(" + i + ")"
			}
			fe.rangeVars(s, i, val, types.Typ[types.Int], u.Elem())
			fe.stmts(s.Body.List)
			w.indent--
			w.ln("}")
			return
		}
		sl, i, n := fe.tmp(), fe.tmp(), fe.tmp()
		w.ln("%s%sfor (let %s = %s, %s = 0, %s = %s === null ? 0 : %s.$length; %s < %s; %s++) {", m, lp, sl, fe.expr(s.X), i, n, sl, sl, i, n, i)
		w.indent++
		val := ""
		if hasVal {
			val = fe.rangeElem(s, fmt.Sprintf("%s!.$array[%s!.$offset + %s]", sl, sl, i), u.Elem())
		}
		fe.rangeVars(s, i, val, types.Typ[types.Int], u.Elem())
		fe.stmts(s.Body.List)
		w.indent--
		w.ln("}")
	case *types.Array:
		arr, i := fe.tmp(), fe.tmp()
		src := "null"
		// With at most one iteration variable and a constant length, Go does
		// not evaluate the range expression (for i := range *nilPtr is fine).
		if hasVal || hasCallOrRecv(s.X) {
			src = fe.expr(s.X)
			if _, isPtr := xt.Underlying().(*types.Pointer); !isPtr && hasVal {
				src = fe.pe.copyExpr(src, xt, fe.tp) // the range expression is a copy
			}
		}
		w.ln("%s%sfor (let %s = %s, %s = 0; %s < %d; %s++) {", m, lp, arr, src, i, i, u.Len(), i)
		w.indent++
		val := ""
		if hasVal {
			val = fe.rangeElem(s, fmt.Sprintf("%s[%s]", arr, i), u.Elem())
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
			val = fe.rangeElem(s, v, u.Elem())
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
		w.ln("const %s = %s;", r, fe.awaitOp("$rt.recv("+ch+")"))
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

// terminates reports whether a statement list ends in a statement that is
// lowered to a JS jump (return, break, continue), so no break follows it.
func terminates(list []ast.Stmt) bool {
	if len(list) == 0 {
		return false
	}
	switch s := list[len(list)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return s.Tok != token.FALLTHROUGH
	case *ast.BlockStmt:
		return terminates(s.List)
	case *ast.IfStmt:
		if s.Else == nil || !terminates(s.Body.List) {
			return false
		}
		return terminates([]ast.Stmt{s.Else})
	case *ast.ForStmt: // for {} without a break of it never ends normally
		return s.Cond == nil && !breaks(s.Body, "", true)
	case *ast.LabeledStmt:
		if f, ok := s.Stmt.(*ast.ForStmt); ok {
			return f.Cond == nil && !breaks(f.Body, s.Label.Name, true)
		}
	}
	return false
}

// breaks reports whether n contains a break of the loop whose body it is:
// an unlabeled break outside nested loops, switches and selects (when
// direct), or a break of label.
func breaks(n ast.Node, label string, direct bool) bool {
	found := false
	ast.Inspect(n, func(c ast.Node) bool {
		if found {
			return false
		}
		switch c := c.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BranchStmt:
			if c.Tok == token.BREAK && ((c.Label == nil && direct) || (c.Label != nil && c.Label.Name == label)) {
				found = true
			}
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if c != n {
				found = label != "" && breaks(c, label, false)
				if !found {
					return false
				}
			}
		}
		return true
	})
	return found
}

func hasDefaultCase(s *ast.SelectStmt) bool {
	for _, c := range s.Body.List {
		if c.(*ast.CommClause).Comm == nil {
			return true
		}
	}
	return false
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// rangeFunc lowers range-over-func (Go 1.23 iterators) to a call with a
// yield callback. break and continue of this loop and return (also from
// nested loops, switches and labeled statements in the body) become the
// callback's result. A body that may block is an async callback; the
// analysis treats it as a function value of the yield type, so the
// iterators that may call it await their yield calls.
//
// A state variable reproduces Go's checks on misbehaving iterators: calling
// yield again after the body returned false, after the loop exited or after
// the body panicked, and returning normally after recovering a body panic.
func (fe *funcEmitter) rangeFunc(s *ast.RangeStmt, label string, sig *types.Signature) {
	w := fe.w
	async := fe.pe.prog.RangeBodyAsync(s)
	branches, exits, ok := fe.rangeFuncBranches(s, label, async)
	if !ok {
		return
	}
	yield := sig.Params().At(0).Type().Underlying().(*types.Signature)
	var params []string
	var ptypes []types.Type
	for i := 0; i < yield.Params().Len(); i++ {
		params = append(params, fe.tmp())
		ptypes = append(ptypes, yield.Params().At(i).Type())
	}
	rf := &rangeFuncCtx{ret: fe.tmp(), retv: fe.tmp(), state: fe.tmp(), branches: branches, exits: map[*ast.BranchStmt]int{}}
	for i, b := range exits {
		rf.exits[b] = i + 2
	}
	returns := containsReturn(s.Body)
	if returns {
		w.ln("%slet %s = 0, %s: any, %s = 0;", fe.mark(s), rf.ret, rf.retv, rf.state)
	} else {
		w.ln("%slet %s = 0, %s = 0;", fe.mark(s), rf.ret, rf.state)
	}
	var decls []string
	for i, p := range params {
		decls = append(decls, p+": "+fe.ts(ptypes[i]))
	}
	call := fmt.Sprintf("%s((%s): boolean => {", fe.expr(s.X), strings.Join(decls, ", "))
	if async {
		call = fmt.Sprintf("%s(async (%s): Promise<boolean> => {", fe.expr(s.X), strings.Join(decls, ", "))
	}
	awaited := async || fe.pe.prog.RangeBlocks(fe.info, s)
	if awaited {
		call = "await " + call
	}
	w.ln("%s", call)
	w.indent++
	w.ln("if (%s !== 0) $rt.rangeError(%s);", rf.state, rf.state)
	w.ln("%s = 3;", rf.state) // in the body: a panic leaves it at 3
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
	outer, outerAsync := fe.rangeFn, fe.async
	fe.rangeFn, fe.async = rf, async
	fe.stmts(s.Body.List)
	fe.rangeFn, fe.async = outer, outerAsync
	if !terminates(s.Body.List) {
		w.ln("%s", rf.next(true))
	}
	w.indent--
	w.ln("});")
	if awaited && fe.pe.prog.TracksGoroutines {
		w.ln("$rt.resumeG($g, 0);")
	}
	w.ln("if (%s === 3) $rt.rangeError(4);", rf.state)
	w.ln("%s = 2;", rf.state)
	switch {
	case fe.sig == nil || !returns:
	case outer != nil:
		// A return in a range-over-func body nested in another one.
		w.ln("if (%s === 1) { %s = 1; %s = %s; %s }", rf.ret, outer.ret, outer.retv, rf.retv, outer.next(false))
	case fe.hasDefer:
		w.ln("if (%s === 1) { %s; break $body; }", rf.ret, fe.assignResults(rf.retv))
	case fe.sig.Results().Len() > 0:
		w.ln("if (%s === 1) return %s;", rf.ret, rf.retv)
	default:
		w.ln("if (%s === 1) return;", rf.ret)
	}
	// Branches to labels outside the loop are taken after the call.
	for i, b := range exits {
		w.ln("if (%s === %d) {", rf.ret, i+2)
		w.indent++
		fe.stmt(b, "")
		w.indent--
		w.ln("}")
	}
}

// rangeFuncCtx is the range-over-func body being lowered.
type rangeFuncCtx struct {
	ret, retv, state string
	// branches are the break/continue statements that target the loop;
	// exits those that target an enclosing statement, by their ret code.
	branches map[*ast.BranchStmt]bool
	exits    map[*ast.BranchStmt]int
}

// next is the callback's return statement continuing (or stopping) the loop.
func (rf *rangeFuncCtx) next(more bool) string {
	if more {
		return fmt.Sprintf("return (%s = 0, true);", rf.state)
	}
	return fmt.Sprintf("return (%s = 1, false);", rf.state)
}

// rangeFuncReturn lowers a return statement in a range-over-func body: the
// results are stored and the loop stops; rangeFunc returns after the call.
func (fe *funcEmitter) rangeFuncReturn(s *ast.ReturnStmt) {
	rf := fe.rangeFn
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
	fe.w.ln("%s%s = 1; %s = %s; %s", fe.mark(s), rf.ret, rf.retv, v, rf.next(false))
}

// rangeFuncBranches finds the branch statements in the body of range loop s
// that target it and those that leave it for an enclosing labeled
// statement, and reports unsupported forms in the body (those in nested
// range-over-func bodies are reported when lowering them).
func (fe *funcEmitter) rangeFuncBranches(s *ast.RangeStmt, label string, async bool) (map[*ast.BranchStmt]bool, []*ast.BranchStmt, bool) {
	ok := true
	nested := 0
	bad := func(n ast.Node, format string, args ...any) {
		if nested == 0 {
			fe.errorf(n.Pos(), format, args...)
			ok = false
		}
	}
	var exits []*ast.BranchStmt
	inner := map[string]bool{} // labels declared in the body
	ast.Inspect(s.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.LabeledStmt:
			inner[fe.labelName(n.Label)] = true
		}
		return true
	})
	targets := map[*ast.BranchStmt]bool{}
	// brk/cont: an unlabeled break/continue at this point targets s.
	var visit func(root ast.Node, brk, cont bool)
	visit = func(root ast.Node, brk, cont bool) {
		ast.Inspect(root, func(n ast.Node) bool {
			if n == root || n == nil {
				return true
			}
			switch n := n.(type) {
			case *ast.FuncLit:
				return false // its own function: may block, defer, return
			case *ast.ForStmt:
				visit(n, false, false)
				return false
			case *ast.RangeStmt:
				switch under(fe.info.TypeOf(n.X)).(type) {
				case *types.Signature:
					nested++
					visit(n, false, false)
					nested--
					return false
				}
				visit(n, false, false)
				return false
			case *ast.SwitchStmt, *ast.TypeSwitchStmt:
				visit(n, false, cont)
				return false
			case *ast.SelectStmt:
				// One with a default case never waits, in a body that
				// is not async too.
				if !async && !hasDefaultCase(n) {
					bad(n, "select in a range-over-func body is not supported here")
				}
				visit(n, false, cont)
				return false
			case *ast.DeferStmt:
				// The body's closure sees the enclosing function's $d, so
				// the call runs when that function returns, as in Go.
			case *ast.BranchStmt:
				switch {
				case n.Tok == token.GOTO:
					// A goto within the body stays in its closure; one out
					// of it is taken after the call, like a labeled break.
					if !inner[fe.labelName(n.Label)] {
						exits = append(exits, n)
					}
				case n.Tok == token.FALLTHROUGH:
				case n.Label != nil:
					switch name := fe.labelName(n.Label); {
					case name == label:
						targets[n] = true
					case !inner[name]:
						exits = append(exits, n)
					}
				case n.Tok == token.BREAK && brk, n.Tok == token.CONTINUE && cont:
					targets[n] = true
				}
			}
			return true
		})
	}
	visit(s.Body, true, true)
	return targets, exits, ok
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
		if fall || terminates(body) {
			w.ln("}")
		} else {
			w.ln("} break;")
		}
	}
	w.indent--
	w.ln("}")
}

// concreteCase reports whether a type switch case of type t matches exactly
// the values whose dynamic type is t (not an interface, type parameter or nil).
func concreteCase(t types.Type) bool {
	if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return false
	}
	return !isTypeParam(t) && !types.IsInterface(t) && !hasTypeParam(t)
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
	// Cases of concrete types compare the dynamic type, loaded once.
	xt := ""
	for _, cl := range clauses {
		for _, e := range cl.(*ast.CaseClause).List {
			if t := fe.info.TypeOf(e); concreteCase(t) && xt == "" {
				xt = fe.forceTmp(fmt.Sprintf("%s === null ? null : %s.t", xv, xv))
			}
		}
	}
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
			} else if concreteCase(t) {
				conds = append(conds, fmt.Sprintf("%s === %s", xt, fe.desc(t)))
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
			if isTypeParam(t) {
				val = fmt.Sprintf("$rt.unboxAs(%s, %s)", fe.desc(t), xv)
			} else if !isIface(t) {
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
	sc := fmt.Sprintf("$rt.select([%s], %v)", strings.Join(real, ", "), hasDefault)
	if !hasDefault {
		sc = fe.awaitOp(sc)
	}
	w.ln("%sconst %s = %s;", fe.mark(s), sel, sc)
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
		if terminates(cc.Body) {
			w.ln("}")
		} else {
			w.ln("} break;")
		}
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
		fe.setResults(m, vals)
		w.ln("%sbreak $body;", m)
		return
	}
	if fe.named && len(vals) > 0 {
		// Go assigns the result variables before returning; an escaped
		// pointer to one observes the value.
		fe.setResults(m, vals)
		vals = nil
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
					direct := len(sel.Index()) == 1
					switch {
					case !direct:
						// A promoted method: evaluate the method value now,
						// which dereferences embedded pointers and
						// interfaces and copies a value receiver.
						set(f, fe.expr(f))
					case direct && isIface(xt):
						// x.M on a nil interface panics at the defer statement.
						set(f.X, "$rt.deref("+fe.expr(f.X)+")")
					case direct && xPtr && !ptrRecv:
						// p.M with a value receiver evaluates *p now: a nil
						// p panics here, and later writes through p are not
						// seen by the deferred call.
						base := xt.Underlying().(*types.Pointer).Elem()
						v := "$rt.deref(" + fe.expr(f.X) + ")"
						if isAggregate(base) {
							set(f.X, fe.pe.copyExpr(v, base, fe.tp))
						} else {
							set(f.X, "$rt.cell<"+fe.ts(base)+">("+v+".v)")
						}
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
	if fe.callBlocks(call) || strings.Contains(body, "await ") {
		if fe.pe.prog.TracksGoroutines {
			// The thunk runs on the goroutine that calls it: a new one for
			// a go statement.
			return "async () => { const $g = $rt.getG(); " + body + "; }"
		}
		return "async () => { " + body + "; }"
	}
	return "() => { " + body + "; }"
}

// hasCallOrRecv reports whether e contains a function call or a channel
// receive, which makes len(e) non-constant for an array operand.
func hasCallOrRecv(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			found = true
		case *ast.UnaryExpr:
			if n.Op == token.ARROW {
				found = true
			}
		case *ast.FuncLit:
			return false
		}
		return !found
	})
	return found
}

// containsReturn reports whether s has a return statement outside nested
// function literals.
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

// A gotoTarget is a label of a goto state machine: goto sets the machine's
// state variable to n and continues its loop.
type gotoTarget struct {
	state, loop string
	n           int
}

// A breakable is an enclosing for, range, switch or select statement (loop
// reports whether continue applies to it), or a goto state machine.
type breakable struct {
	label   string
	loop    bool
	machine bool
}

// pushBreakable records the breakable statement s with JS label label. A
// statement gets a label even without a Go one if an unlabelled break or
// continue of it is inside a goto state machine, so that it can name it.
func (fe *funcEmitter) pushBreakable(s ast.Stmt, label string, loop bool) string {
	if label == "" && containsGoto(s) && fe.branchesFromMachine(s, loop) {
		label = "J" + fe.tmp()
	}
	fe.breakables = append(fe.breakables, breakable{label: label, loop: loop})
	return label
}

func (fe *funcEmitter) popBreakable() { fe.breakables = fe.breakables[:len(fe.breakables)-1] }

// branchLabel returns the JS label an unlabelled break (or continue) must
// name because a goto state machine lies between it and its statement, or "".
func (fe *funcEmitter) branchLabel(cont bool) string {
	crossed := false
	for i := len(fe.breakables) - 1; i >= 0; i-- {
		b := fe.breakables[i]
		switch {
		case b.machine:
			crossed = true
		case cont && !b.loop:
		case crossed:
			return b.label
		default:
			return ""
		}
	}
	return ""
}

func containsGoto(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(c ast.Node) bool {
		switch c := c.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BranchStmt:
			found = found || c.Tok == token.GOTO
		}
		return !found
	})
	return found
}

// gotosTo calls f for each goto in n (outside function literals) whose
// target is label.
func (fe *funcEmitter) gotosTo(n ast.Node, label types.Object, f func(*ast.BranchStmt)) {
	ast.Inspect(n, func(c ast.Node) bool {
		switch c := c.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BranchStmt:
			if c.Tok == token.GOTO && fe.info.Uses[c.Label] == label {
				f(c)
			}
		}
		return true
	})
}

// backwardGoto reports whether a goto jumps back to a label of list: from
// the labelled statement itself or from a later one.
func (fe *funcEmitter) backwardGoto(list []ast.Stmt) bool {
	for k, s := range list {
		ls, ok := s.(*ast.LabeledStmt)
		if !ok {
			continue
		}
		label := fe.info.Defs[ls.Label]
		found := false
		for _, t := range list[k:] {
			fe.gotosTo(t, label, func(*ast.BranchStmt) { found = true })
		}
		if found {
			return true
		}
	}
	return false
}

// gotoMachine lowers a statement list with a backward goto to a state
// machine: a loop around a switch on a state variable, with a case at each
// goto target. Falling through the cases runs the statements in order; goto
// sets the state and continues the loop:
//
//	let $1 = 0;
//	M$1: for (;;) {
//		switch ($1) {
//		case 0:
//			...
//		case 1: // L:
//			...; $1 = 1; continue M$1; // goto L
//		}
//		break;
//	}
//
// The switch body is entered afresh on every jump, so the variables the
// list declares are declared before the loop (Go forbids jumping over a
// declaration into its scope, so they are always assigned before use).
// Jumping back over a declaration therefore reuses its variable, where Go
// would make a new one; this is only observable through closures created
// before the jump.
func (fe *funcEmitter) gotoMachine(list []ast.Stmt) {
	w := fe.w
	state := fe.tmp()
	loop := "M" + state
	if fe.gotoTargets == nil {
		fe.gotoTargets = map[types.Object]gotoTarget{}
	}
	if fe.hoisted == nil {
		fe.hoisted = map[*types.Var]bool{}
	}
	cases := map[int]int{} // list index -> state
	for k, s := range list {
		ls, ok := s.(*ast.LabeledStmt)
		if !ok {
			continue
		}
		label := fe.info.Defs[ls.Label]
		found := false
		for _, t := range list {
			fe.gotosTo(t, label, func(*ast.BranchStmt) { found = true })
		}
		if found {
			cases[k] = len(cases) + 1
			fe.gotoTargets[label] = gotoTarget{state: state, loop: loop, n: cases[k]}
		}
	}
	for _, s := range list {
		for _, v := range fe.declaredVars(s) {
			n := fe.declare(v)
			fe.hoisted[v] = true
			if fe.boxed(v) {
				w.ln("let %s!: $rt.Cell<%s>;", n, fe.ts(v.Type()))
			} else {
				w.ln("let %s!: %s;", n, fe.ts(v.Type()))
			}
		}
	}
	w.ln("let %s = 0;", state)
	w.ln("%s: for (;;) {", loop)
	w.indent++
	w.ln("switch (%s) {", state)
	w.ln("case 0:")
	fe.breakables = append(fe.breakables, breakable{machine: true})
	w.indent++
	for k, s := range list {
		if n, ok := cases[k]; ok {
			w.indent--
			w.ln("case %d:", n)
			w.indent++
		}
		fe.stmt(s, "")
	}
	fe.popBreakable()
	w.indent--
	w.ln("}")
	w.ln("break;")
	w.indent--
	w.ln("}")
}

// declaredVars returns the variables a statement of a list declares in the
// list's scope.
func (fe *funcEmitter) declaredVars(s ast.Stmt) []*types.Var {
	var vars []*types.Var
	add := func(id *ast.Ident) {
		if v, ok := fe.info.Defs[id].(*types.Var); ok && v.Name() != "_" {
			vars = append(vars, v)
		}
	}
	switch s := s.(type) {
	case *ast.LabeledStmt:
		return fe.declaredVars(s.Stmt)
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for _, l := range s.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					add(id)
				}
			}
		}
	case *ast.DeclStmt:
		if gd, ok := s.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			for _, spec := range gd.Specs {
				for _, id := range spec.(*ast.ValueSpec).Names {
					add(id)
				}
			}
		}
	}
	return vars
}

// branchesFromMachine reports whether an unlabelled break of s (or continue,
// if it is a loop) lies in a statement list lowered to a goto state machine.
func (fe *funcEmitter) branchesFromMachine(s ast.Stmt, loop bool) bool {
	found := false
	var visit func(n ast.Node, brk, cont, machine bool)
	visitList := func(list []ast.Stmt, brk, cont, machine bool) {
		machine = machine || fe.backwardGoto(list)
		for _, st := range list {
			visit(st, brk, cont, machine)
		}
	}
	visit = func(n ast.Node, brk, cont, machine bool) {
		if found || n == nil {
			return
		}
		switch n := n.(type) {
		case *ast.FuncLit:
			return
		case *ast.BranchStmt:
			if n.Label == nil && machine && (n.Tok == token.BREAK && brk || n.Tok == token.CONTINUE && cont) {
				found = true
			}
			return
		case *ast.BlockStmt:
			visitList(n.List, brk, cont, machine)
			return
		case *ast.CaseClause:
			visitList(n.Body, brk, cont, machine)
			return
		case *ast.CommClause:
			visitList(n.Body, brk, cont, machine)
			return
		case *ast.ForStmt, *ast.RangeStmt:
			if n != s {
				return // break and continue inside are its own
			}
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if n != s {
				brk = false
			}
		}
		ast.Inspect(n, func(c ast.Node) bool {
			if c == n {
				return true
			}
			visit(c, brk, cont, machine)
			return false
		})
	}
	visit(s, true, loop, false)
	return found
}

// litRecoverTok is the recover token (funcEmitter.recoverTok) of a function
// literal: its package, file and position, which do not depend on the order
// in which packages were loaded, so a module's code is the same in every
// build (internal/build caches it). A file on disk is named by its base
// name; the natives replacements and patches keep their whole name, since a
// patch has the base name of the file it patches.
func (pe *pkgEmitter) litRecoverTok(lit *ast.FuncLit) string {
	p := pe.prog.Fset.PositionFor(lit.Pos(), false)
	name := p.Filename
	if filepath.IsAbs(name) {
		name = filepath.Base(name)
	}
	return fmt.Sprintf("func@%s/%s:%d:%d", pe.pkg.PkgPath, name, p.Line, p.Column)
}

// deferredTok is the recover token of the function a defer statement calls,
// so that recover() recovers only in that function (Go's "called directly by
// a deferred function"), or "" when the callee is only known at run time
// (function values, interface methods): then any recover() during the
// deferred call recovers.
func (fe *funcEmitter) deferredTok(call *ast.CallExpr) string {
	fun := unparen(call.Fun)
	if tv, ok := fe.info.Types[fun]; ok && tv.IsType() {
		return ""
	}
	switch f := fun.(type) {
	case *ast.FuncLit:
		return fe.pe.litRecoverTok(f)
	case *ast.IndexExpr:
		fun = unparen(f.X)
	case *ast.IndexListExpr:
		fun = unparen(f.X)
	}
	var obj types.Object
	switch f := fun.(type) {
	case *ast.Ident:
		obj = fe.info.Uses[f]
	case *ast.SelectorExpr:
		if sel, ok := fe.info.Selections[f]; ok {
			if sel.Kind() == types.FieldVal {
				return ""
			}
			obj = sel.Obj()
		} else {
			obj = fe.info.Uses[f.Sel]
		}
	}
	switch o := obj.(type) {
	case *types.Builtin:
		return "builtin " + o.Name()
	case *types.Func:
		if recv := o.Signature().Recv(); recv != nil && isIface(recv.Type()) {
			return "" // an interface method: the dynamic type's method
		}
		return o.Origin().FullName()
	}
	return ""
}

// stringToBytesArg returns s if e is the conversion []byte(s) of a string.
func stringToBytesArg(info *types.Info, e ast.Expr) (ast.Expr, bool) {
	conv, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 {
		return nil, false
	}
	if tv, ok := info.Types[ast.Unparen(conv.Fun)]; !ok || !tv.IsType() || !isByteSlice(tv.Type) {
		return nil, false
	}
	if b, ok := under(info.TypeOf(conv.Args[0])).(*types.Basic); !ok || b.Info()&types.IsString == 0 {
		return nil, false
	}
	return conv.Args[0], true
}

// scalarRangeVar returns the fields that the body of range loop s reads of
// its value variable v if the loop loads them into locals instead of
// copying the element (see scalarRangeVars).
func (fe *funcEmitter) scalarRangeVar(s *ast.RangeStmt, v *types.Var) ([]int, bool) {
	if v == nil || fe.sharedRangeVars[v] || s.Tok != token.DEFINE {
		return nil, false
	}
	fields, ok := fe.pe.scalar[v]
	return fields, ok
}

// rangeElem returns the value of the element elem (of type t) that range
// loop s assigns to its value variable: a copy, unless the loop reads its
// fields into locals.
func (fe *funcEmitter) rangeElem(s *ast.RangeStmt, elem string, t types.Type) string {
	if id, ok := s.Value.(*ast.Ident); ok {
		if v, ok := fe.info.Defs[id].(*types.Var); ok {
			if _, ok := fe.scalarRangeVar(s, v); ok {
				return elem
			}
		}
	}
	return fe.pe.copyExpr(elem, t, fe.tp)
}

// defineFields declares a local for each field of struct value x (the
// element of the iteration) that the loop reads of v.
func (fe *funcEmitter) defineFields(v *types.Var, x string, fields []int) {
	if len(fields) == 0 {
		return
	}
	st := v.Type().Underlying().(*types.Struct)
	if len(fields) > 1 {
		t := fe.tmp()
		fe.w.ln("const %s = %s;", t, x)
		x = t
	}
	names := map[int]string{}
	for _, i := range fields {
		f := st.Field(i)
		n := fe.declareName(jsName(v.Name()) + "$" + jsName(f.Name()))
		names[i] = n
		fe.w.ln("let %s: %s = %s.%s;", n, fe.ts(f.Type()), x, fieldProp(st, i))
	}
	fe.fieldLocals[v] = names
}
