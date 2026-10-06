package lower

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"regexp"
	"strconv"
	"strings"
)

func isNumericConst(v constant.Value) bool {
	k := v.Kind()
	return k == constant.Int || k == constant.Float || k == constant.Complex
}

// numericTypeSet reports whether t is a type parameter whose type set has
// only numeric types.
func numericTypeSet(t types.Type) bool {
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return false
	}
	found := false
	var numeric func(t types.Type) bool
	numeric = func(t types.Type) bool {
		switch u := t.Underlying().(type) {
		case *types.Basic:
			found = true
			return u.Info()&types.IsNumeric != 0
		case *types.Union:
			for i := 0; i < u.Len(); i++ {
				if !numeric(u.Term(i).Type()) {
					return false
				}
			}
			return true
		case *types.Interface:
			for i := 0; i < u.NumEmbeddeds(); i++ {
				if !numeric(u.EmbeddedType(i)) {
					return false
				}
			}
			return true
		}
		return false
	}
	return numeric(tp.Constraint()) && found
}

// constT lowers numeric constant v of type parameter type t: its
// representation (Number, BigInt, complex) is that of the type argument.
func (fe *funcEmitter) constT(v constant.Value, t types.Type) string {
	if v.Kind() == constant.Complex {
		if constant.Sign(constant.Imag(v)) != 0 {
			return fmt.Sprintf("$rt.constT(%s, %s)", fe.desc(t), constLit(v, types.Typ[types.Complex128]))
		}
		v = constant.Real(v) // representable in every type of t's type set
	}
	if v.Kind() == constant.Int {
		if i, exact := constant.Int64Val(v); !exact || i > 1<<53 || i < -(1<<53) {
			return fmt.Sprintf("$rt.constT(%s, %sn)", fe.desc(t), v.ExactString())
		}
	}
	return fmt.Sprintf("$rt.constT(%s, %s)", fe.desc(t), constLit(v, types.Typ[types.UntypedFloat]))
}

// expr lowers an expression to a JS expression string. The result denotes
// the Go value without copying it; contexts that need Go copy semantics
// use valueOf.
func (fe *funcEmitter) expr(e ast.Expr) string {
	if s, ok := fe.override[e]; ok {
		return s
	}
	if tv, ok := fe.info.Types[e]; ok && tv.Value != nil {
		if isTypeParam(tv.Type) && tv.Value.Kind() != constant.String && tv.Value.Kind() != constant.Bool {
			return fe.constT(tv.Value, tv.Type)
		}
		return constLit(tv.Value, tv.Type)
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return "(" + fe.expr(e.X) + ")"
	case *ast.Ident:
		return fe.ident(e)
	case *ast.BasicLit:
		fe.errorf(e.Pos(), "internal: non-constant literal")
		return "undefined"
	case *ast.FuncLit:
		return fe.funcLit(e)
	case *ast.CompositeLit:
		return fe.compositeLit(e)
	case *ast.SelectorExpr:
		return fe.selector(e)
	case *ast.IndexExpr:
		return fe.index(e)
	case *ast.IndexListExpr:
		return fe.funcInstance(e, e.X)
	case *ast.SliceExpr:
		return fe.sliceExpr(e)
	case *ast.TypeAssertExpr:
		return fmt.Sprintf("%s$rt.assert(%s, %s, %s)", fe.mark(e), fe.expr(e.X), fe.desc(fe.info.TypeOf(e.X)), fe.desc(fe.info.TypeOf(e.Type)))
	case *ast.CallExpr:
		return fe.call(e)
	case *ast.StarExpr:
		t := fe.info.TypeOf(e)
		p := fe.expr(e.X)
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP {
			return fmt.Sprintf("$rt.load(%s, %s)", fe.desc(t), p)
		}
		if isAggregate(t) {
			return fe.mark(e) + "$rt.deref(" + p + ")"
		}
		return fe.mark(e) + nilChecked(p) + ".v"
	case *ast.UnaryExpr:
		return fe.unary(e)
	case *ast.BinaryExpr:
		return fe.binary(e)
	case *ast.KeyValueExpr:
		fe.errorf(e.Pos(), "internal: unexpected key/value expression")
	}
	fe.errorf(e.Pos(), "unsupported expression %T", e)
	return "undefined"
}

func (fe *funcEmitter) ident(e *ast.Ident) string {
	obj := fe.info.Uses[e]
	if obj == nil {
		obj = fe.info.Defs[e]
	}
	switch obj := obj.(type) {
	case *types.Nil:
		return "null"
	case *types.Var:
		if fe.isSplit(obj) {
			return fe.splitRead(obj)
		}
		return fe.varRef(obj)
	case *types.Func:
		if inst, ok := fe.info.Instances[e]; ok {
			return fe.genericFuncValue(fe.nameOf(obj), inst.TypeArgs)
		}
		return fe.nameOf(obj)
	case *types.Const:
		return constLit(obj.Val(), obj.Type())
	}
	fe.errorf(e.Pos(), "unsupported identifier %s", e.Name)
	return "undefined"
}

func (fe *funcEmitter) typeArgList(targs *types.TypeList) []string {
	var out []string
	for i := 0; i < targs.Len(); i++ {
		out = append(out, fe.desc(targs.At(i)))
	}
	return out
}

func (fe *funcEmitter) genericFuncValue(fn string, targs *types.TypeList) string {
	return fmt.Sprintf("((...a: any[]) => (%s as any)(%s, ...a))", fn, strings.Join(fe.typeArgList(targs), ", "))
}

func (fe *funcEmitter) funcInstance(e ast.Expr, x ast.Expr) string {
	if sel, ok := unparen(x).(*ast.SelectorExpr); ok && fe.info.Selections[sel] != nil {
		// A generic method value or expression with explicit type
		// arguments (s.M[int]): they are recorded on the selector.
		return fe.selector(sel)
	}
	id := identOf(x)
	inst, ok := fe.info.Instances[id]
	if !ok {
		fe.errorf(e.Pos(), "unsupported instantiation")
		return "undefined"
	}
	return fe.genericFuncValue(fe.nameOf(fe.info.Uses[id]), inst.TypeArgs)
}

// isFresh reports whether e denotes a newly created value that no other Go
// variable can observe, so aggregate copies can be skipped.
func (fe *funcEmitter) isFresh(e ast.Expr) bool {
	switch e := unparen(e).(type) {
	case *ast.CompositeLit, *ast.TypeAssertExpr, *ast.FuncLit:
		return true
	case *ast.UnaryExpr:
		return e.Op == token.ARROW
	case *ast.CallExpr:
		if tv, ok := fe.info.Types[unparen(e.Fun)]; ok && tv.IsType() {
			return len(e.Args) == 1 && fe.isFresh(e.Args[0])
		}
		return true
	}
	return false
}

// valueOf lowers e for use as a value of type target: aggregates are copied
// (Go value semantics) and concrete values are boxed into interfaces.
func (fe *funcEmitter) valueOf(e ast.Expr, target types.Type) string {
	src := fe.info.TypeOf(e)
	if b, ok := src.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return "null"
	}
	s := fe.expr(e)
	if !fe.isFresh(e) {
		s = fe.pe.copyExpr(s, src, fe.tp)
	}
	return fe.convert(s, src, target)
}

// convert applies implicit conversions (currently: boxing into interfaces).
func (fe *funcEmitter) convert(s string, from, to types.Type) string {
	if to == nil || from == nil {
		return s
	}
	if b, ok := from.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return "null"
	}
	if isIface(to) && !isIface(from) {
		if _, isTP := types.Unalias(to).(*types.TypeParam); isTP {
			return s
		}
		if b, ok := from.(*types.Basic); ok && b.Info()&types.IsUntyped != 0 {
			from = types.Default(from)
		}
		if _, isTP := types.Unalias(from).(*types.TypeParam); isTP {
			// The type argument may be an interface type, which $rt.box
			// leaves as it is.
			return fmt.Sprintf("$rt.box(%s, %s)", fe.desc(from), s)
		}
		if p, ok := under(from).(*types.Pointer); ok {
			if _, ok := under(p.Elem()).(*types.Struct); ok {
				return fmt.Sprintf("$rt.ptrIface(%s, %s)", fe.desc(from), s)
			}
		}
		if b := fe.pe.boxOf(from); b != nil {
			return fe.boxValue(s, from, b)
		}
		return fmt.Sprintf("new $rt.Iface(%s, %s)", fe.desc(from), s)
	}
	return s
}

func (fe *funcEmitter) convertCopy(s string, from, to types.Type) string {
	return fe.convert(fe.pe.copyExpr(s, from, fe.tp), from, to)
}

// fieldBase resolves a field selector (including promoted fields) to the JS
// object holding the field and the property name.
// nilChecked is pointer p as the object of a property access (p!.f, p!.v,
// p![i]). The access itself checks for nil: JS throws a TypeError reading or
// writing a property of null, which recover sees as Go's nil dereference
// runtime error (toPanic), as do exported functions called from JS
// (exportWrapper). An explicit check ($rt.deref) would cost V8 its fast
// property access: NBody's loops run at half speed with one.
func nilChecked(p string) string {
	if simpleRef.MatchString(stripMarks(p)) {
		return p + "!"
	}
	return "(" + p + ")!"
}

func (fe *funcEmitter) fieldBase(e *ast.SelectorExpr) (string, string) {
	sel := fe.info.Selections[e]
	obj := fe.expr(e.X)
	t := fe.info.TypeOf(e.X)
	path := sel.Index()
	for i, idx := range path {
		base, isPtr := derefType(t)
		if isPtr {
			obj = nilChecked(obj) // also an embedded pointer
		}
		st := base.Underlying().(*types.Struct)
		prop := fieldProp(st, idx)
		if i == len(path)-1 {
			return obj, prop
		}
		obj = obj + "." + prop
		t = st.Field(idx).Type()
	}
	return obj, ""
}

func (fe *funcEmitter) selector(e *ast.SelectorExpr) string {
	sel, ok := fe.info.Selections[e]
	if !ok {
		// Qualified identifier pkg.Name.
		return fe.mark(e) + fe.ident(e.Sel)
	}
	switch sel.Kind() {
	case types.FieldVal:
		if name, ok := fe.promotedField(e); ok {
			return fe.mark(e) + name // see fieldRun
		}
		if id, ok := e.X.(*ast.Ident); ok {
			if v, ok := fe.info.Uses[id].(*types.Var); ok && fe.fieldLocals[v] != nil {
				names := fe.fieldLocals[v]
				return fe.mark(e) + names[sel.Index()[0]] // see scalarRangeVars
			}
		}
		obj, prop := fe.fieldBase(e)
		return fe.mark(e) + obj + "." + prop
	case types.MethodVal:
		if slow, locker := fe.pe.prog.WaitLockVal(e); locker {
			// l.Lock bound to the waiting variant (lockcheck.go).
			fn := sel.Obj().(*types.Func)
			l := fe.convert(fe.expr(e.X), fe.info.TypeOf(e.X), fn.Signature().Recv().Type())
			return fmt.Sprintf("%s((r: any) => () => %s(null, r))($rt.deref(%s))", fe.mark(e), fe.pe.methodFuncName(slow), l)
		} else if slow != nil {
			_, recv, _ := fe.methodTarget(e, sel)
			return fmt.Sprintf("%s((r: any) => () => %s(r))(%s)", fe.mark(e), fe.pe.methodFuncName(slow), recv)
		}
		fn, recv, iface := fe.methodTarget(e, sel)
		r := fe.tmp()
		if iface { // a nil interface panics when the method value is taken
			return fmt.Sprintf("%s((%s: any) => (...a: any[]) => $rt.icall(%s, %s, ...a))($rt.deref(%s))", fe.mark(e), r, r, jsString(methodKey(sel.Obj().(*types.Func))), recv)
		}
		// Method value: the receiver is evaluated (and copied) now.
		if fnT := sel.Obj().(*types.Func); !isPtrRecv(fnT) {
			recv = fe.pe.copyExpr(recv, fnT.Signature().Recv().Type(), fe.tp)
		}
		// fn is "F(" plus dictionaries; the call spreads the arguments, so F
		// is called untyped.
		fn = "(" + strings.Replace(fn, "(", " as any)(", 1)
		if dialerMethods[sel.Obj().(*types.Func).FullName()] {
			// net/http's patched RoundTrip recognizes a net.Dialer's dialers
			// (dialerOf in natives.ts).
			return fmt.Sprintf("((%s: any) => Object.assign((...a: any[]) => %s%s, ...a), { $dialer: %s }))(%s)", r, fn, r, r, recv)
		}
		return fmt.Sprintf("((%s: any) => (...a: any[]) => %s%s, ...a))(%s)", r, fn, r, recv)
	case types.MethodExpr:
		if slow, locker := fe.pe.prog.WaitLockVal(e); locker {
			return fmt.Sprintf("((r: any) => %s(null, r))", fe.pe.methodFuncName(slow))
		} else if slow != nil {
			return fmt.Sprintf("((r: any) => %s(r))", fe.pe.methodFuncName(slow))
		}
		fn := sel.Obj().(*types.Func)
		recvT := sel.Recv()
		if isTypeParam(recvT) { // T.M for a type parameter: dispatch on the type argument
			return fmt.Sprintf("((r: any, ...a: any[]) => $rt.icall($rt.box(%s, r), %s, ...a))", fe.desc(recvT), jsString(methodKey(fn)))
		}
		if isIface(recvT) {
			return fmt.Sprintf("((r: any, ...a: any[]) => $rt.icall(r, %s, ...a))", jsString(methodKey(fn)))
		}
		// The same function as the method table entry: it follows embedded
		// fields (promoted methods) and adjusts the receiver.
		targs := ""
		if inst, ok := fe.info.Instances[e.Sel]; ok { // T.M[int]: the method's own type arguments come last
			n, m := inst.TypeArgs.Len(), fn.Signature().TypeParams().Len()
			for i := n - m; i < n; i++ {
				targs += fe.desc(inst.TypeArgs.At(i)) + ", "
			}
		}
		return "(" + fe.pe.methodWrapper(recvT, sel, fe.tp, targs) + ")"
	}
	return "undefined"
}

// dialerMethods are the methods whose method values record their receiver
// as $dialer.
var dialerMethods = map[string]bool{
	"(*net.Dialer).Dial":        true,
	"(*net.Dialer).DialContext": true,
}

func isPtrRecv(fn *types.Func) bool {
	_, ok := fn.Signature().Recv().Type().(*types.Pointer)
	return ok
}

// methodTarget resolves a method selector x.M to (function prefix, receiver
// argument, isInterface). For concrete methods the prefix is "Fn(" plus any
// type-argument dictionaries, so the call is prefix + recv + args.
func (fe *funcEmitter) methodTarget(e *ast.SelectorExpr, sel *types.Selection) (string, string, bool) {
	fn := sel.Obj().(*types.Func)
	path := sel.Index()
	recv := fe.expr(e.X)
	t := fe.info.TypeOf(e.X)
	parent, parentProp := "", ""
	for _, idx := range path[:len(path)-1] {
		base, _ := derefType(t)
		st := base.Underlying().(*types.Struct)
		parent, parentProp = recv, fieldProp(st, idx)
		recv = recv + "." + parentProp
		t = st.Field(idx).Type()
	}
	if isIface(fn.Signature().Recv().Type()) {
		if _, ok := types.Unalias(t).(*types.TypeParam); ok {
			// A constraint method on a type-parameter-typed value: the
			// value is unboxed (erasure), so dispatch through the
			// dictionary's method table by boxing it with its type.
			recv = fmt.Sprintf("$rt.box(%s, %s)", fe.desc(t), recv)
		}
		return "", recv, true
	}
	wantPtr := isPtrRecv(fn)
	base, havePtr := derefType(t)
	switch {
	case wantPtr && !havePtr:
		if !isAggregate(base) {
			if len(path) == 1 {
				recv = fe.addrOf(e.X)
			} else {
				recv = fmt.Sprintf("$rt.fieldPtr(%s, %s)", parent, jsString(parentProp))
			}
		}
	case !wantPtr && havePtr:
		if isAggregate(base) {
			recv = "$rt.deref(" + recv + ")" // the object is the pointer
		} else {
			recv += ".v"
		}
	}
	targs := fe.pe.recvTypeArgs(base, fe.tp)
	if inst, ok := fe.info.Instances[e.Sel]; ok && fn.Signature().TypeParams().Len() > 0 {
		// Generic method (Go 1.27): method type arguments follow the
		// receiver's type arguments.
		n := inst.TypeArgs.Len()
		m := fn.Signature().TypeParams().Len()
		for i := n - m; i < n; i++ {
			targs += fe.desc(inst.TypeArgs.At(i)) + ", "
		}
	}
	return fe.pe.methodFuncName(fn) + "(" + targs, recv, false
}

// addrOf lowers &e.
func (fe *funcEmitter) addrOf(e ast.Expr) string {
	t := fe.info.TypeOf(e)
	switch x := unparen(e).(type) {
	case *ast.Ident:
		v := fe.info.Uses[x].(*types.Var)
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP && fe.boxed(v) {
			return fmt.Sprintf("$rt.tpAddr(%s, %s)", fe.desc(t), fe.nameOf(v))
		}
		if isAggregate(t) {
			return fe.nameOf(v)
		}
		if !fe.boxed(v) {
			fe.errorf(e.Pos(), "internal: address of unboxed variable %s", v.Name())
		}
		return fe.nameOf(v)
	case *ast.SelectorExpr:
		if _, ok := fe.info.Selections[x]; !ok {
			v := fe.info.Uses[x.Sel].(*types.Var)
			return fe.nameOf(v)
		}
		obj, prop := fe.fieldBase(x)
		if isTypeParam(t) {
			return fmt.Sprintf("%s$rt.tpFieldAddr(%s, %s, %s)", fe.mark(x), fe.desc(t), obj, jsString(prop))
		}
		if isAggregate(t) {
			return obj + "." + prop
		}
		return fmt.Sprintf("%s$rt.fieldPtr(%s, %s)", fe.mark(x), obj, jsString(prop))
	case *ast.IndexExpr:
		xt := fe.info.TypeOf(x.X)
		if _, ok := xt.Underlying().(*types.Slice); ok {
			if isTypeParam(t) {
				return fmt.Sprintf("%s$rt.tpSliceElemAddr(%s, %s, %s)", fe.mark(x), fe.desc(t), fe.expr(x.X), fe.intNumber(x.Index))
			}
			if isAggregate(t) {
				if s, i, ok := fe.checkedIndex(x); ok {
					return fe.mark(x) + fe.elem(x, s, i)
				}
				return fe.mark(x) + sliceIndex(fe.expr(x.X), fe.intNumber(x.Index))
			}
			return fmt.Sprintf("%s$rt.sliceElemPtr(%s, %s)", fe.mark(x), fe.expr(x.X), fe.intNumber(x.Index))
		}
		if isTypeParam(t) {
			a := fe.expr(x.X)
			if _, isPtr := under(xt).(*types.Pointer); isPtr {
				a = "$rt.deref(" + a + ")"
			}
			return fmt.Sprintf("%s$rt.tpArrayElemAddr(%s, %s, %s)", fe.mark(x), fe.desc(t), a, fe.intNumber(x.Index))
		}
		if isAggregate(t) {
			return fmt.Sprintf("%s[%s]", fe.expr(x.X), fe.arrayIndex(x))
		}
		return fmt.Sprintf("%s$rt.arrayElemPtr(%s, %s)", fe.mark(x), fe.expr(x.X), fe.intNumber(x.Index))
	case *ast.StarExpr: // &*p is p, but a nil p still panics
		return fe.mark(x) + "$rt.deref(" + fe.expr(x.X) + ")"
	case *ast.CompositeLit:
		if isAggregate(t) {
			return fe.compositeLit(x)
		}
		return "$rt.cell<" + fe.ts(t) + ">(" + fe.compositeLit(x) + ")"
	}
	fe.errorf(e.Pos(), "unsupported address-of operand %T", e)
	return "undefined"
}

func (fe *funcEmitter) arrayIndex(x *ast.IndexExpr) string {
	if tv, ok := fe.info.Types[x.Index]; ok && tv.Value != nil {
		return fe.intNumber(x.Index) // constant indices are bounds-checked by go/types
	}
	xt := fe.info.TypeOf(x.X)
	base, _ := derefType(xt)
	n := base.Underlying().(*types.Array).Len()
	if max, ok := indexBound(fe.info, x.Index); (ok && max < n) || fe.pe.inBounds[x] {
		return fe.intNumber(x.Index)
	}
	return fmt.Sprintf("$rt.arrayIndex(%d, %s)", n, fe.intNumber(x.Index))
}

// indexBound returns the largest value of the integer expression e that its
// type or a constant mask allows, if that is small: a byte indexes a
// [256]T table without a bounds check.
func indexBound(info *types.Info, e ast.Expr) (int64, bool) {
	e = ast.Unparen(e)
	if b, ok := under(info.TypeOf(e)).(*types.Basic); ok {
		switch b.Kind() {
		case types.Uint8:
			return 1<<8 - 1, true
		case types.Uint16:
			return 1<<16 - 1, true
		}
	}
	if be, ok := e.(*ast.BinaryExpr); ok && be.Op == token.AND {
		for _, m := range []ast.Expr{be.X, be.Y} {
			if tv, ok := info.Types[m]; ok && tv.Value != nil && constant.Sign(tv.Value) >= 0 {
				if v, ok := constant.Int64Val(constant.ToInt(tv.Value)); ok {
					return v, true
				}
			}
		}
	}
	return 0, false
}

func (fe *funcEmitter) index(e *ast.IndexExpr) string {
	if _, ok := fe.info.Instances[identOf(e.X)]; ok {
		return fe.funcInstance(e, e.X)
	}
	xt := fe.info.TypeOf(e.X)
	m := fe.mark(e)
	switch u := under(xt).(type) {
	case *types.Basic:
		if x, i, ok := fe.checkedIndex(e); ok {
			return m + x + ".charCodeAt(" + i + ")"
		}
		if x, set, i, ok := fe.checkedIndexTemp(e); ok {
			return m + set + x + ".charCodeAt(" + i + "))"
		}
		x, set, i := fe.indexTemp(fe.expr(e.X), fe.intNumber(e.Index))
		return m + set + strIndex(x, i) + closeIf(set)
	case *types.Slice:
		if x, i, ok := fe.checkedIndex(e); ok {
			return fe.byteBoolLoad(e.X, m+fe.elem(e, x, i))
		}
		if x, set, i, ok := fe.checkedIndexTemp(e); ok {
			return fe.byteBoolLoad(e.X, m+set+fe.elem(e, x, i)+")")
		}
		x, set, i := fe.indexTemp(fe.expr(e.X), fe.intNumber(e.Index))
		return fe.byteBoolLoad(e.X, m+set+sliceIndex(x, i)+closeIf(set))
	case *types.Map:
		return fmt.Sprintf("%s$rt.mapGet(%s, %s, %s)", m, fe.expr(e.X), fe.valueOf(e.Index, u.Key()), fe.zeroFn(u.Elem()))
	case *types.Array:
		return fmt.Sprintf("%s%s[%s]", m, fe.expr(e.X), fe.arrayIndex(e))
	case *types.Pointer: // *array: the array object, nil-checked
		return fmt.Sprintf("%s%s[%s]", m, nilChecked(fe.expr(e.X)), fe.arrayIndex(e))
	case *types.Interface:
		// Type parameter without a core type, e.g. ~string | ~[]byte: the
		// representation is chosen at run time.
		return fmt.Sprintf("%s$rt.indexAny(%s, %s)", m, fe.expr(e.X), fe.intNumber(e.Index))
	}
	fe.errorf(e.Pos(), "unsupported index expression on %s", xt)
	return "undefined"
}

func (fe *funcEmitter) sliceExpr(e *ast.SliceExpr) string {
	opt := func(x ast.Expr) string {
		if x == nil {
			return "undefined"
		}
		return fe.intNumber(x)
	}
	xt := fe.info.TypeOf(e.X)
	m := fe.mark(e)
	switch u := under(xt).(type) {
	case *types.Basic:
		return fmt.Sprintf("%s$rt.substr(%s, %s, %s)", m, fe.expr(e.X), opt(e.Low), opt(e.High))
	case *types.Slice:
		return fmt.Sprintf("%s$rt.slice(%s, %s, %s, %s)", m, fe.expr(e.X), opt(e.Low), opt(e.High), opt(e.Max))
	case *types.Array, *types.Pointer:
		_ = u
		return fmt.Sprintf("%s$rt.sliceArray(%s, %s, %s, %s)", m, fe.expr(e.X), opt(e.Low), opt(e.High), opt(e.Max))
	case *types.Interface: // type parameter without core type
		return fmt.Sprintf("%s$rt.sliceAny(%s, %s, %s)", m, fe.expr(e.X), opt(e.Low), opt(e.High))
	}
	fe.errorf(e.Pos(), "unsupported slice expression on %s", xt)
	return "undefined"
}

func (fe *funcEmitter) compositeLit(e *ast.CompositeLit) string {
	if isTypeParam(fe.info.TypeOf(e)) {
		// A literal of a type parameter's core type (P with core type *S
		// in []P{{f: 1}}) is a value of P only in Go's type system.
		return "(" + fe.compositeLitOf(e) + " as any)"
	}
	return fe.compositeLitOf(e)
}

func (fe *funcEmitter) compositeLitOf(e *ast.CompositeLit) string {
	t := fe.info.TypeOf(e)
	if p, ok := under(t).(*types.Pointer); ok { // elided &T in nested literals
		t = p.Elem()
	}
	m := fe.mark(e)
	switch u := under(t).(type) {
	case *types.Struct:
		vals := make([]string, u.NumFields())
		for i := range vals {
			vals[i] = fe.zero(u.Field(i).Type())
		}
		var elems []elemVal
		var promoted []string // property paths of promoted fields, slots NumFields+k
		for i, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				name := kv.Key.(*ast.Ident).Name
				found := false
				for j := 0; j < u.NumFields(); j++ {
					if u.Field(j).Name() == name {
						elems = append(elems, elemVal{j, fe.valueOf(kv.Value, u.Field(j).Type())})
						found = true
					}
				}
				if !found { // a promoted field (A{b: x} with b in an embedded B)
					obj, path, _ := types.LookupFieldOrMethod(t, false, fe.pe.pkg.Types, name)
					st, props := u, ""
					for _, k := range path {
						props += "." + fieldProp(st, k)
						ft, _ := derefType(st.Field(k).Type())
						st, _ = under(ft).(*types.Struct)
					}
					elems = append(elems, elemVal{u.NumFields() + len(promoted), fe.valueOf(kv.Value, obj.Type())})
					promoted = append(promoted, props)
				}
				continue
			}
			v := fe.valueOf(el, u.Field(i).Type())
			if u.Field(i).Name() == "_" { // evaluated, but a blank field stays zero
				switch x := unparen(el).(type) {
				case *ast.Ident, *ast.BasicLit, *ast.FuncLit:
					v = fe.zero(u.Field(i).Type())
				default:
					if tv := fe.info.Types[x]; tv.Value != nil {
						v = fe.zero(u.Field(i).Type())
					} else {
						v = "(" + v + ", " + fe.zero(u.Field(i).Type()) + ")"
					}
				}
			}
			elems = append(elems, elemVal{i, v})
		}
		pre := fe.spillOutOfOrder(elems)
		var sets []string
		for _, ev := range elems {
			if ev.slot >= len(vals) {
				sets = append(sets, "$o"+promoted[ev.slot-len(vals)]+" = "+ev.val)
				continue
			}
			vals[ev.slot] = ev.val
		}
		class := ""
		if isTypeParam(t) { // the type argument's class (it may be a defined type)
			class = "(" + fe.desc(t) + ".ctor)"
		} else {
			class = fe.pe.structClass(t)
		}
		lit := fmt.Sprintf("%snew %s(%s)", m, class, strings.Join(vals, ", "))
		if len(sets) > 0 {
			lit = fmt.Sprintf("(($o: any) => (%s, $o))(%s)", strings.Join(sets, ", "), lit)
		}
		return wrapPre(pre, lit)
	case *types.Array:
		pre, vals := fe.indexedElems(e, u.Elem(), int(u.Len()))
		return wrapPre(pre, m+"["+strings.Join(vals, ", ")+"]")
	case *types.Slice:
		pre, vals := fe.indexedElems(e, u.Elem(), -1)
		return wrapPre(pre, m+"$rt.sliceLit<"+fe.ts(u.Elem())+">(["+strings.Join(vals, ", ")+"])")
	case *types.Map:
		var kvs []string
		for _, el := range e.Elts {
			kv := el.(*ast.KeyValueExpr)
			kvs = append(kvs, fe.valueOf(kv.Key, u.Key()), fe.valueOf(kv.Value, u.Elem()))
		}
		return fmt.Sprintf("%s$rt.mapLit(%s, [%s])", m, fe.desc(u.Key()), strings.Join(kvs, ", "))
	}
	fe.errorf(e.Pos(), "unsupported composite literal of type %s", t)
	return "undefined"
}

// indexedElems lowers array/slice literal elements, honouring explicit
// indices; n < 0 means "as long as needed".
func (fe *funcEmitter) indexedElems(e *ast.CompositeLit, elem types.Type, n int) (string, []string) {
	var elems []elemVal
	idx, max := 0, 0
	for _, el := range e.Elts {
		v := el
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			c := fe.info.Types[kv.Key].Value
			i, _ := constant.Int64Val(constant.ToInt(c))
			idx = int(i)
			v = kv.Value
		}
		elems = append(elems, elemVal{idx, fe.valueOf(v, elem)})
		idx++
		if idx > max {
			max = idx
		}
	}
	pre := fe.spillOutOfOrder(elems)
	vals := map[int]string{}
	for _, ev := range elems {
		vals[ev.slot] = ev.val
	}
	if n < 0 {
		n = max
	}
	out := make([]string, n)
	for i := range out {
		if v, ok := vals[i]; ok {
			out[i] = v
		} else {
			out[i] = fe.zero(elem)
		}
	}
	return pre, out
}

// elemVal is a composite literal element: its lowered value and the slot
// (field or index) it initializes.
type elemVal struct {
	slot int
	val  string
}

// spillOutOfOrder keeps Go's lexical evaluation order for literal elements
// whose slots are not in source order (S{B: f(), A: g()}): it evaluates them
// into temporaries, returning the comma-expression prefix that does so.
func (fe *funcEmitter) spillOutOfOrder(elems []elemVal) string {
	ordered := true
	for i := 1; i < len(elems); i++ {
		if elems[i].slot < elems[i-1].slot {
			ordered = false
		}
	}
	if ordered {
		return ""
	}
	var names, parts []string
	for i := range elems {
		t := fe.tmp()
		names = append(names, t)
		parts = append(parts, t+" = "+elems[i].val)
		elems[i].val = t
	}
	fe.w.ln("let %s;", strings.Join(names, ", "))
	return strings.Join(parts, ", ")
}

func wrapPre(pre, s string) string {
	if pre == "" {
		return s
	}
	return "(" + pre + ", " + s + ")"
}

// ---- operators ----

type intInfo struct {
	bits   int
	signed bool
	big    bool // int64 or uint64: a BigInt
}

// bigOperand types an operand of a BigInt operator for TypeScript: a value
// read through `any` (a pointer's field, an interface's value) would make
// a - b a number. Names and literals are typed already.
func bigOperand(s string) string {
	if simpleOperand.MatchString(s) {
		return s
	}
	return "(" + s + " as bigint)"
}

var simpleOperand = regexp.MustCompile(`^(-?[0-9]+n|[A-Za-z_$][A-Za-z0-9_$]*)$`)

// bigWrap truncates a BigInt to the width and signedness of ii.
func bigWrap(s string, ii intInfo) string {
	if ii.signed {
		return fmt.Sprintf("BigInt.asIntN(%d, %s)", ii.bits, s)
	}
	return fmt.Sprintf("BigInt.asUintN(%d, %s)", ii.bits, s)
}

// bigUnwrap returns the operand of s if s is bigWrap(e, ii), possibly typed
// as bigint, and s otherwise.
func bigUnwrap(s string, ii intInfo) string {
	if in, ok := strings.CutSuffix(s, " as bigint)"); ok && strings.HasPrefix(in, "(") {
		if e := bigUnwrap(in[1:], ii); e != in[1:] {
			return e
		}
	}
	fn := "BigInt.asUintN"
	if ii.signed {
		fn = "BigInt.asIntN"
	}
	// Keep the position markers the call starts with.
	lead := 0
	for lead < len(s) && s[lead] == markStart {
		end := strings.IndexByte(s[lead:], markEnd)
		if end < 0 {
			return s
		}
		lead += end + 1
	}
	marks, s0 := s[:lead], s
	s = s[lead:]
	prefix := fmt.Sprintf("%s(%d, ", fn, ii.bits)
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return s0
	}
	// The call's parenthesis must close at the end of s, skipping those
	// in string literals.
	depth := 0
	var quote rune
	escaped := false
	for i, c := range s[len(fn):] {
		switch {
		case quote != 0:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == quote:
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 && len(fn)+i != len(s)-1 {
				return s0
			}
		}
	}
	if depth != 0 || quote != 0 {
		return s0
	}
	return marks + s[len(prefix):len(s)-1]
}

func intKind(t types.Type) (intInfo, bool) {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return intInfo{}, false
	}
	switch b.Kind() {
	case types.Int8:
		return intInfo{8, true, false}, true
	case types.Int16:
		return intInfo{16, true, false}, true
	case types.Int32, types.UntypedRune:
		return intInfo{32, true, false}, true
	case types.Int64:
		return intInfo{64, true, true}, true
	case types.Int, types.UntypedInt:
		return intInfo{64, true, false}, true
	case types.Uint8:
		return intInfo{8, false, false}, true
	case types.Uint16:
		return intInfo{16, false, false}, true
	case types.Uint32:
		return intInfo{32, false, false}, true
	case types.Uint64:
		return intInfo{64, false, true}, true
	case types.Uint, types.Uintptr:
		return intInfo{64, false, false}, true
	}
	return intInfo{}, false
}

// wrap truncates an integer result to its Go width. int64 and uint64 are
// BigInts (see bigWrap); int, uint and uintptr are JS numbers, exact up to
// 2^53 and not wrapped (documented gap).
func wrap(s string, ii intInfo) string {
	if ii.big {
		return bigWrap(s, ii)
	}
	switch {
	case ii.bits == 32 && ii.signed:
		return "((" + s + ") | 0)"
	case ii.bits == 32:
		return "((" + s + ") >>> 0)"
	case ii.bits == 16 && ii.signed:
		return "((" + s + ") << 16 >> 16)"
	case ii.bits == 16:
		return "((" + s + ") & 0xffff)"
	case ii.bits == 8 && ii.signed:
		return "((" + s + ") << 24 >> 24)"
	case ii.bits == 8:
		return "((" + s + ") & 0xff)"
	}
	// Parenthesized so that an operand position (an op-assignment's
	// right side, an identity conversion) keeps Go's grouping.
	return "(" + s + ")"
}

func isTypeParam(t types.Type) bool {
	_, ok := types.Unalias(t).(*types.TypeParam)
	return ok
}

// basicTypeParam reports whether t is a type parameter whose type set holds
// only basic types other than complex ones (as with cmp.Ordered): its
// values are JS numbers, BigInts, strings or booleans, which need no copy
// and compare with === as Go compares them (NaN included).
func basicTypeParam(t types.Type) bool {
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return false
	}
	iface, ok := tp.Constraint().Underlying().(*types.Interface)
	return ok && basicTypeSet(iface, map[*types.Interface]bool{})
}

// basicTypeSet reports whether the type set of iface is restricted to
// non-complex basic types: one of its embedded elements (the type set is
// their intersection) has only such terms.
func basicTypeSet(iface *types.Interface, seen map[*types.Interface]bool) bool {
	if seen[iface] {
		return false
	}
	seen[iface] = true
	basic := func(t types.Type) bool {
		b, ok := t.Underlying().(*types.Basic)
		return ok && b.Info()&types.IsComplex == 0 && b.Kind() != types.UnsafePointer && b.Kind() != types.UntypedNil
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		switch e := types.Unalias(iface.EmbeddedType(i)).(type) {
		case *types.Union:
			all := e.Len() > 0
			for j := 0; j < e.Len(); j++ {
				if !basic(e.Term(j).Type()) {
					all = false
				}
			}
			if all {
				return true
			}
		default:
			if in, ok := e.Underlying().(*types.Interface); ok {
				if basicTypeSet(in, seen) {
					return true
				}
			} else if basic(e) {
				return true
			}
		}
	}
	return false
}

func isComplex(t types.Type) bool {
	b, ok := under(t).(*types.Basic)
	return ok && b.Info()&types.IsComplex != 0
}

func isComplex64(t types.Type) bool {
	b, ok := under(t).(*types.Basic)
	return ok && b.Kind() == types.Complex64
}

func isFloat32(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Float32
}

// arith lowers a binary arithmetic/bitwise operator on lowered operands of
// type t.
func (fe *funcEmitter) arith(op token.Token, a, b string, t types.Type) string {
	if isTypeParam(t) {
		// The operand kind (and so wrapping, integer division, string
		// concatenation) is that of the type argument.
		return fmt.Sprintf("$rt.arithT(%s, %q, %s, %s)", fe.desc(t), op.String(), a, b)
	}
	if ii, ok := intKind(t); ok && ii.big {
		// Written inline so that engines that compile asIntN(64, ...) to
		// machine arithmetic (V8) can.
		switch op {
		case token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT:
			// These are congruent modulo 2^64 to the result of operands
			// that are, so truncating the whole expression once makes
			// operands' truncations redundant. The bitwise operators
			// stay in range for operands of one signedness, but V8
			// computes on 64-bit integers only under an asIntN or
			// asUintN: x ^ (x << 13n) unwrapped allocates a BigInt.
			a, b = bigOperand(bigUnwrap(a, ii)), bigOperand(bigUnwrap(b, ii))
			if op == token.AND_NOT {
				return bigWrap(a+" & ~"+b, ii)
			}
			return bigWrap(a+" "+op.String()+" "+b, ii)
		}
		a, b := bigOperand(a), bigOperand(b)
		switch op {
		case token.QUO:
			return bigWrap("$rt.divBig("+a+", "+b+")", ii)
		case token.REM:
			return "$rt.modBig(" + a + ", " + b + ")"
		}
	}
	if ii, ok := intKind(t); ok {
		switch op {
		case token.ADD, token.SUB:
			return wrap(a+" "+op.String()+" "+b, ii)
		case token.MUL:
			if ii.bits == 32 {
				return wrap("$rt.imul("+a+", "+b+")", ii)
			}
			return wrap(a+" * "+b, ii)
		case token.QUO:
			if constDivisor(b) {
				// The quotient of integers below 2^53 rounds toward zero
				// exactly; + 0 turns -0 into 0 where wrap does not.
				q := "$rt.trunc(" + a + " / " + b + ")"
				if ii.bits == 64 && ii.signed {
					return "(" + q + " + 0)"
				}
				return wrap(q, ii)
			}
			if pa, ra, pb, rb, ok := fe.reuse2(a, b); ok {
				// Inline, as with a constant divisor; $rt.div is a call
				// that large functions may not inline.
				q := "(" + pa + pb + rb + " === 0 ? $rt.divZero() : $rt.trunc(" + ra + " / " + rb + ") + 0)"
				if ii.bits == 64 && ii.signed {
					return q
				}
				return wrap(q, ii)
			}
			return wrap("$rt.div("+a+", "+b+")", ii)
		case token.REM:
			if constDivisor(b) {
				r := "((" + a + ") % " + b + ")"
				if ii.bits == 64 && ii.signed {
					return "(" + r + " + 0)"
				}
				return wrap(r, ii)
			}
			if pa, ra, pb, rb, ok := fe.reuse2(a, b); ok {
				r := "(" + pa + pb + rb + " === 0 ? $rt.divZero() : " + ra + " % " + rb + " + 0)"
				if ii.bits == 64 && ii.signed {
					return r
				}
				return wrap(r, ii)
			}
			return wrap("$rt.mod("+a+", "+b+")", ii)
		case token.AND, token.OR, token.XOR, token.AND_NOT:
			if ii.bits == 64 && op == token.AND && (smallMask(a) || smallMask(b)) {
				// The bits a mask below 2^31 keeps are those of the
				// int32 that & converts the other operand to.
				return "(" + a + " & " + b + ")"
			}
			if ii.bits == 64 {
				fn := map[token.Token]string{token.AND: "and64", token.OR: "or64", token.XOR: "xor64", token.AND_NOT: "andNot64"}[op]
				return "$rt." + fn + "(" + a + ", " + b + ")"
			}
			if op == token.AND_NOT {
				return wrap(a+" & ~"+b, ii)
			}
			return wrap(a+" "+op.String()+" "+b, ii)
		}
	}
	if b0, ok := t.Underlying().(*types.Basic); ok && b0.Info()&types.IsString != 0 {
		return "(" + a + " + " + b + ")"
	}
	if isComplex(t) {
		fn := map[token.Token]string{token.ADD: "cadd", token.SUB: "csub", token.MUL: "cmul", token.QUO: "cdiv"}[op]
		c := "$rt." + fn + "(" + a + ", " + b + ")"
		if isComplex64(t) {
			return "$rt.c64(" + c + ")"
		}
		return c
	}
	s := "(" + a + " " + op.String() + " " + b + ")"
	if isFloat32(t) {
		return "$rt.fround" + s
	}
	return s
}

// shiftCount lowers a shift count to a JS number.
func (fe *funcEmitter) shiftCount(e ast.Expr) string {
	return fe.intNumber(e)
}

// intNumber lowers an integer expression used as a JS number (an index, a
// length, a shift count): BigInts are converted (a value beyond 2^53 is out
// of range either way).
func (fe *funcEmitter) intNumber(e ast.Expr) string {
	if tv, ok := fe.info.Types[e]; ok && tv.Value != nil {
		// A constant, possibly untyped float or complex (x << 1.0).
		if v := constant.ToInt(tv.Value); v.Kind() == constant.Int {
			return v.ExactString()
		}
	}
	t := fe.info.TypeOf(e)
	if isTypeParam(t) {
		return "$rt.intNumber(" + fe.expr(e) + ")"
	}
	if isBig(t) {
		return "Number(" + fe.expr(e) + ")"
	}
	return fe.expr(e)
}

func (fe *funcEmitter) shift(op token.Token, a, n string, t types.Type) string {
	if isTypeParam(t) {
		return fmt.Sprintf("$rt.shiftT(%s, %v, %s, %s)", fe.desc(t), op == token.SHL, a, n)
	}
	ii, _ := intKind(t)
	if c, err := strconv.Atoi(n); err == nil && c >= 0 {
		if s, ok := constShift(op, a, c, ii); ok {
			return s
		}
	}
	if ii.big {
		if op == token.SHL {
			return fmt.Sprintf("$rt.shlBig(%s, %s, %v)", a, n, ii.signed)
		}
		return fmt.Sprintf("$rt.shrBig(%s, %s)", a, n)
	}
	if ii.bits == 64 {
		if op == token.SHL {
			return fmt.Sprintf("$rt.shl64(%s, %s, %v)", a, n, ii.signed)
		}
		return fmt.Sprintf("$rt.shr64(%s, %s, %v)", a, n, ii.signed)
	}
	if op == token.SHL {
		return wrap(fmt.Sprintf("$rt.shl32(%s, %s)", a, n), ii)
	}
	return wrap(fmt.Sprintf("$rt.shr32(%s, %s, %v)", a, n, ii.signed), ii)
}

// constShift lowers a shift of a by the constant count n without the
// runtime's checks, where JS has the operator: n below the width (the
// 32-bit operators use n mod 32), and for int, uint and uintptr (numbers)
// right shifts only.
func constShift(op token.Token, a string, n int, ii intInfo) (string, bool) {
	switch {
	case ii.big && n < 64:
		if op == token.SHL {
			return bigWrap(fmt.Sprintf("(%s) << %dn", a, n), ii), true
		}
		return fmt.Sprintf("((%s) >> %dn)", a, n), true
	case ii.bits == 64:
		if op == token.SHL || n >= 64 {
			return "", false
		}
		return fmt.Sprintf("$rt.floor((%s) / %s)", a, strconv.FormatFloat(math.Ldexp(1, n), 'f', -1, 64)), true
	case n < 32:
		if op == token.SHL {
			return wrap(fmt.Sprintf("(%s) << %d", a, n), ii), true
		}
		if ii.signed {
			return fmt.Sprintf("((%s) >> %d)", a, n), true
		}
		return fmt.Sprintf("((%s) >>> %d)", a, n), true
	}
	return "", false
}

// constDivisor reports whether the lowered divisor b is a non-zero integer
// literal.
// smallMask reports whether the lowered operand s is an integer constant
// in [0, 2^31).
func smallMask(s string) bool {
	v, err := strconv.ParseInt(strings.Trim(s, "()"), 10, 64)
	return err == nil && v >= 0 && v < 1<<31
}

func constDivisor(b string) bool {
	v, err := strconv.ParseFloat(strings.Trim(b, "()"), 64)
	return err == nil && v != 0 && v == math.Trunc(v)
}

// eqExpr compares two lowered operands of (identical or assignable) types.
func (fe *funcEmitter) eqExpr(a string, at types.Type, b string, bt types.Type) string {
	ai, bi := isIface(at), isIface(bt)
	switch {
	case ai && bi:
		return fmt.Sprintf("$rt.ifaceEq(%s, %s)", a, b)
	case ai && !bi:
		if isNil(bt) {
			return a + " === null"
		}
		return fmt.Sprintf("$rt.ifaceEq(%s, %s)", a, fe.convert(b, bt, at))
	case bi && !ai:
		if isNil(at) {
			return b + " === null"
		}
		return fmt.Sprintf("$rt.ifaceEq(%s, %s)", fe.convert(a, at, bt), b)
	}
	if _, isTP := types.Unalias(at).(*types.TypeParam); isTP {
		if basicTypeParam(at) {
			return "(" + a + " === " + b + ")"
		}
		return fmt.Sprintf("$rt.equal(%s, %s, %s)", fe.desc(at), a, b)
	}
	if isAggregate(at) {
		return fmt.Sprintf("$rt.equal(%s, %s, %s)", fe.desc(at), a, b)
	}
	if isComplex(at) {
		return "$rt.ceq(" + a + ", " + b + ")"
	}
	if p, ok := under(at).(*types.Pointer); ok && zeroSize(p.Elem()) && !isNil(at) && !isNil(bt) {
		// Like gc, all zero-size values share one address (runtime.zerobase),
		// so two non-nil pointers to them are equal.
		return "$rt.zeroSizePtrEq(" + a + ", " + b + ")"
	}
	if isUnsafePointer(at) && isUnsafePointer(bt) {
		// Pointer arithmetic makes other objects for the same address.
		return "$rt.unsafePtrEq(" + a + ", " + b + ")"
	}
	return "(" + a + " === " + b + ")"
}

// zeroSize reports whether values of t occupy no memory, like struct{} or
// [0]int.
func zeroSize(t types.Type) bool {
	if isTypeParam(t) {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Array:
		return u.Len() == 0 || zeroSize(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !zeroSize(u.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

func isNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

// equal compares a lowered switch tag with a case expression.
func (fe *funcEmitter) equal(tag string, tagT types.Type, e ast.Expr) string {
	return fe.eqExpr(tag, tagT, fe.expr(e), fe.info.TypeOf(e))
}

func (fe *funcEmitter) binary(e *ast.BinaryExpr) string {
	if s, ok := fe.splitExpr(e); ok {
		return s
	}
	xt, yt := fe.info.TypeOf(e.X), fe.info.TypeOf(e.Y)
	switch e.Op {
	case token.LAND, token.LOR:
		return "(" + fe.expr(e.X) + " " + e.Op.String() + " " + fe.expr(e.Y) + ")"
	case token.EQL:
		return fe.eqExpr(fe.expr(e.X), xt, fe.expr(e.Y), yt)
	case token.NEQ:
		return "!" + "(" + fe.eqExpr(fe.expr(e.X), xt, fe.expr(e.Y), yt) + ")"
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
		return "(" + fe.cmpOperand(e.X) + " " + e.Op.String() + " " + fe.cmpOperand(e.Y) + ")"
	case token.SHL, token.SHR:
		return fe.mark(e) + fe.shift(e.Op, fe.expr(e.X), fe.shiftCount(e.Y), fe.info.TypeOf(e))
	}
	return fe.mark(e) + fe.arith(e.Op, fe.expr(e.X), fe.expr(e.Y), fe.info.TypeOf(e))
}

// cmpOperand lowers an operand of an ordered comparison. uint and uintptr
// are numbers that do not wrap, so uint(i) of a negative i stays negative;
// converted for a comparison (the bounds check idiom uint(i) < uint(len(s)))
// it is moved above every int, as the wrapped value would be.
func (fe *funcEmitter) cmpOperand(x ast.Expr) string {
	s := fe.expr(x)
	call, ok := unparen(x).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return s
	}
	if tv, ok := fe.info.Types[call.Fun]; !ok || !tv.IsType() {
		return s
	}
	to, ok := intKind(fe.info.TypeOf(x))
	from, ok2 := intKind(fe.info.TypeOf(call.Args[0]))
	if !ok || !ok2 || to.signed || to.big || to.bits != 64 || !from.signed || from.big {
		return s
	}
	if tv := fe.info.Types[call.Args[0]]; tv.Value != nil {
		return s // a constant conversion is representable
	}
	return "$rt.ucmp(" + s + ")"
}

func (fe *funcEmitter) unary(e *ast.UnaryExpr) string {
	if s, ok := fe.splitExpr(e); ok {
		return s
	}
	t := fe.info.TypeOf(e)
	switch e.Op {
	case token.AND:
		return fe.addrOf(e.X)
	case token.ARROW:
		return fe.mark(e) + fe.recvExpr(fe.expr(e.X)) + "[0]"
	case token.NOT:
		return "!" + fe.expr(e.X)
	case token.ADD:
		return fe.expr(e.X)
	case token.SUB:
		if isTypeParam(t) {
			return fmt.Sprintf("$rt.negT(%s, %s)", fe.desc(t), fe.expr(e.X))
		}
		if ii, ok := intKind(t); ok && ii.big {
			return bigWrap("-"+bigOperand(fe.expr(e.X)), ii)
		} else if ok {
			return wrap("-"+fe.expr(e.X), ii)
		}
		if isFloat32(t) {
			return "$rt.fround(-" + fe.expr(e.X) + ")"
		}
		if isComplex(t) {
			return "$rt.cneg(" + fe.expr(e.X) + ")"
		}
		return "(-" + fe.expr(e.X) + ")"
	case token.XOR:
		if isTypeParam(t) {
			return fmt.Sprintf("$rt.notT(%s, %s)", fe.desc(t), fe.expr(e.X))
		}
		ii, _ := intKind(t)
		if ii.big {
			if ii.signed {
				return "(~" + bigOperand(fe.expr(e.X)) + ")"
			}
			return bigWrap("~"+bigOperand(fe.expr(e.X)), ii)
		}
		if ii.bits == 64 {
			return fmt.Sprintf("$rt.not64(%s, %v)", fe.expr(e.X), ii.signed)
		}
		return wrap("~"+fe.expr(e.X), ii)
	}
	fe.errorf(e.Pos(), "unsupported unary operator %s", e.Op)
	return "undefined"
}

// ---- calls ----

func (fe *funcEmitter) call(e *ast.CallExpr) string {
	fun := unparen(e.Fun)
	if sel, ok := unparen(funcIdent(fun)).(*ast.SelectorExpr); ok && fe.info.Selections[sel] != nil {
		if _, inst := fe.info.Instances[sel.Sel]; inst || sel == fun {
			fun = sel // s.M[int](): the method's type arguments are recorded on M
		}
	}
	if tv, ok := fe.info.Types[fun]; ok && tv.IsType() {
		return fe.conversion(e, tv.Type)
	}
	if id, ok := fun.(*ast.Ident); ok {
		if fe.genYield != nil && fe.info.Uses[id] == fe.genYield {
			return fe.genYieldCall(e)
		}
		if b, ok := fe.info.Uses[id].(*types.Builtin); ok {
			return fe.builtin(e, b.Name())
		}
	}
	if se, ok := fun.(*ast.SelectorExpr); ok {
		if b, ok := fe.info.Uses[se.Sel].(*types.Builtin); ok { // unsafe.X
			return fe.unsafeCall(e, b.Name())
		}
	}
	if s, ok := fe.sprintf(e); ok {
		return s
	}
	if s, ok := fe.jsonCall(e); ok {
		return s
	}
	if s, ok := fe.jsValueCall(e); ok {
		return s
	}
	sig := under(fe.info.TypeOf(e.Fun)).(*types.Signature)
	args := fe.args(e, sig)
	var callee string
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		if sel, ok := fe.info.Selections[f]; ok && sel.Kind() == types.MethodExpr {
			if slow, locker := fe.pe.prog.WaitLock(e); slow != nil { // (*sync.Mutex).Lock(&mu)
				if locker {
					return fe.awaitIf(e, fe.mark(e)+fe.pe.methodFuncName(slow)+"(null, "+args+")")
				}
				return fe.awaitIf(e, fe.mark(e)+fe.pe.methodFuncName(slow)+"("+args+")")
			}
		}
		if sel, ok := fe.info.Selections[f]; ok && sel.Kind() == types.MethodVal {
			if bound, ok := fe.override[f]; ok { // a method value evaluated earlier (defer)
				return fe.awaitIf(e, fe.mark(e)+"("+bound+" as any)("+args+")")
			}
			prefix, recv, iface := fe.methodTarget(f, sel)
			fn := sel.Obj().(*types.Func)
			if slow, locker := fe.pe.prog.WaitLock(e); locker {
				// A type parameter constrained by Locker is boxed as one.
				l := fe.convert(fe.expr(f.X), fe.info.TypeOf(f.X), fn.Signature().Recv().Type())
				return fe.awaitIf(e, fe.mark(e)+fe.pe.methodFuncName(slow)+"(null, "+l+")")
			} else if slow != nil {
				prefix = fe.pe.methodFuncName(slow) + "("
			}
			if iface {
				callee = fe.icall(recv, methodKey(fn), args)
			} else {
				if slow, _ := fe.pe.prog.WaitLock(e); slow == nil && fe.pe.prog.SyncClone(fe.info, e, fe.assume) {
					prefix = strings.TrimSuffix(prefix, "(") + "$sync("
				}
				if args != "" {
					callee = prefix + recv + ", " + args + ")"
				} else {
					callee = prefix + recv + ")"
				}
			}
			return fe.awaitIf(e, fe.mark(e)+callee)
		}
	}
	targs := ""
	if id := identOf(funcIdent(fun)); id != nil {
		if inst, ok := fe.info.Instances[id]; ok {
			if fn, ok := fe.info.Uses[id].(*types.Func); ok {
				callee = fe.nameOf(fn)
				targs = strings.Join(fe.typeArgList(inst.TypeArgs), ", ")
				// TS cannot infer type parameters used only in the result
				// (func New[T any]() []T); pass them.
				var ts []string
				for i := 0; i < inst.TypeArgs.Len(); i++ {
					ts = append(ts, fe.ts(inst.TypeArgs.At(i)))
				}
				callee += "<" + strings.Join(ts, ", ") + ">"
			}
		}
	}
	if callee == "" && fe.pe.prog.SyncClone(fe.info, e, fe.assume) {
		callee = fe.nameOf(staticCallee(fe.info, e)) + "$sync"
	}
	if callee == "" {
		callee = fe.expr(fun)
		if _, ok := fun.(*ast.FuncLit); ok {
			callee = "(" + callee + ")"
		} else if !isStaticFunc(fe.info, fun) {
			// Calling a nil function value panics after the arguments
			// are evaluated.
			callee = "(" + callee + " ?? $rt.nilFunc)"
		}
	}
	all := targs
	if args != "" {
		if all != "" {
			all += ", "
		}
		all += args
	}
	return fe.awaitIf(e, fmt.Sprintf("%s%s(%s)", fe.mark(e), callee, all))
}

// sliceIndex is s[i] for the JS expressions s (a slice) and i. When both are
// plain references or literals, which can be evaluated again, the bounds
// check and the load are inline: then each index expression has an inline
// cache of its own for the backing array, where $rt.index, shared by every
// slice of the program, sees all kinds of arrays and cannot be optimized for
// any. $rt.index still produces the panic. The accesses are typed any: TS
// does not narrow every reference (s may be typed null, or never after it).
func sliceIndex(s, i string) string {
	ss, is := stripMarks(s), stripMarks(i)
	if !reusable(ss) || !reusable(is) {
		return fmt.Sprintf("$rt.index(%s, %s)", s, i)
	}
	return fmt.Sprintf("(%s ? (%[2]s as any).$array[(%[2]s as any).$offset + %[3]s] : $rt.index(%[2]s, %[3]s))", inBounds(ss, is), ss, is)
}

// byteBoolLoad converts load, an element of slice x, to a boolean where a
// Uint8Array backs x (see byteBools).
func (fe *funcEmitter) byteBoolLoad(x ast.Expr, load string) string {
	if id, ok := ast.Unparen(x).(*ast.Ident); ok {
		if v, ok := fe.info.ObjectOf(id).(*types.Var); ok && fe.pe.byteBools[v] {
			return "!!(" + load + ")" // inline: V8 ran a sieve 1.5x slower via a helper
		}
	}
	return load
}

// byteBoolStore converts rhs, a boolean stored into slice x, to 1 or 0 where
// a Uint8Array backs x (see byteBools): V8 stores a number into it faster.
func (fe *funcEmitter) byteBoolStore(x ast.Expr, rhs string) string {
	if id, ok := ast.Unparen(x).(*ast.Ident); ok {
		if v, ok := fe.info.ObjectOf(id).(*types.Var); ok && fe.pe.byteBools[v] {
			switch rhs {
			case "true":
				return "(1 as any)"
			case "false":
				return "(0 as any)"
			}
			return "(((" + rhs + ") ? 1 : 0) as any)"
		}
	}
	return rhs
}

// reuse2 makes the operands a and b of an inline binary operation
// reusable: an operand that cannot be evaluated again is assigned to a
// variable of the function, by the prefixes pa and pb ("t = x, "), which
// keep Go's order. ok is false outside a function body.
func (fe *funcEmitter) reuse2(a, b string) (pa, ra, pb, rb string, ok bool) {
	tmp := func(x string) (string, string) {
		if reusable(stripMarks(x)) {
			return "", stripMarks(x)
		}
		t := fe.declareName("$v")
		fe.temps = append(fe.temps, t)
		return t + " = " + x + ", ", t
	}
	if !fe.inBody {
		return "", "", "", "", false
	}
	pa, ra = tmp(a)
	pb, rb = tmp(b)
	return pa, ra, pb, rb, true
}

// checkedIndex returns the JS slice or string x and index i of e when the
// index is in range by construction (see inBoundsIndices).
func (fe *funcEmitter) checkedIndex(e *ast.IndexExpr) (string, string, bool) {
	if !fe.pe.inBounds[e] {
		return "", "", false
	}
	x, i := stripMarks(fe.expr(e.X)), stripMarks(fe.intNumber(e.Index))
	if !simpleRef.MatchString(x) || !simpleRef.MatchString(i) {
		return "", "", false
	}
	return x, i, true
}

// checkedIndexTemp is checkedIndex for a load whose in-range index is an
// expression (s[x % len(s)]): it is assigned to a variable of the function
// first, "(t = i, " to be closed after the load, so that it is evaluated, and
// may panic dividing by zero, before s is read.
func (fe *funcEmitter) checkedIndexTemp(e *ast.IndexExpr) (string, string, string, bool) {
	if !fe.pe.inBounds[e] || !fe.inBody {
		return "", "", "", false
	}
	x := stripMarks(fe.expr(e.X))
	if !simpleRef.MatchString(x) {
		return "", "", "", false
	}
	t := fe.declareName("$i")
	fe.temps = append(fe.temps, t)
	return x, "(" + t + " = " + fe.intNumber(e.Index) + ", ", t, true
}

// sliceElem is the element s[i] of slice s for an index known to be in range.
func sliceElem(s, i string) string {
	return fmt.Sprintf("(%[1]s as any).$array[(%[1]s as any).$offset + %[2]s]", s, i)
}

// elem is sliceElem for e, s[i], using the header of s loaded before the
// loop if it was (hoistSliceHeaders).
func (fe *funcEmitter) elem(e *ast.IndexExpr, s, i string) string {
	if h, ok := fe.sliceHdr(e.X); ok {
		return h.array + "[" + h.offset + " + " + i + "]"
	}
	return sliceElem(s, i)
}

// sliceHeader names the constants holding the array, offset and length of
// a slice.
type sliceHeader struct{ array, offset, length string }

// sliceHdr returns the header of x loaded before the loop being lowered, if
// x is a variable whose header was.
func (fe *funcEmitter) sliceHdr(x ast.Expr) (sliceHeader, bool) {
	if fe.sliceHdrs == nil {
		return sliceHeader{}, false
	}
	id, ok := ast.Unparen(x).(*ast.Ident)
	if !ok {
		return sliceHeader{}, false
	}
	v, _ := fe.info.Uses[id].(*types.Var)
	h, ok := fe.sliceHdrs[v]
	return h, ok
}

// loadSliceHeader declares the constants holding the header of slice v.
// The array and offset of a nil slice are null and 0: undefined would make
// every o + i in the loop possibly NaN.
func (fe *funcEmitter) loadSliceHeader(v *types.Var) {
	if fe.sliceHdrs == nil {
		fe.sliceHdrs = map[*types.Var]sliceHeader{}
	}
	x := fe.varRef(v)
	h := sliceHeader{fe.tmp(), fe.tmp(), fe.tmp()}
	fe.w.ln("const %[2]s = %[1]s === null ? null : (%[1]s as any).$array, %[3]s = %[1]s === null ? 0 : (%[1]s as any).$offset, %[4]s = %[1]s === null ? 0 : (%[1]s as any).$length;", x, h.array, h.offset, h.length)
	fe.sliceHdrs[v] = h
}

// loadParamHeaders loads the headers of the parameters of sig that
// hdrLoops lists outside function literals at the start of the function
// body, for all its loops.
func (fe *funcEmitter) loadParamHeaders(sig *types.Signature) {
	hl := fe.pe.hdrLoops
	if hl == nil {
		return
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if v := sig.Params().At(i); hl.outer[v] && !fe.boxed(v) {
			fe.loadSliceHeader(v)
		}
	}
}

// hoistSliceHeaders loads the array, offset and length of the slices
// inBoundsIndices found for loop s into constants before it, which the
// in-range elements, len and range clauses of the slices in the loop use: an
// engine reloads them on every iteration of a loop that stores to an
// object, as it cannot tell such a store from one to the slice header. A
// block around the loop scopes the constants; the returned function closes
// it.
func (fe *funcEmitter) hoistSliceHeaders(s ast.Stmt) func() {
	var vars []*types.Var
	if hl := fe.pe.hdrLoops; hl != nil {
		for _, v := range hl.loops[s] {
			if _, ok := fe.sliceHdrs[v]; !ok && !fe.boxed(v) {
				vars = append(vars, v)
			}
		}
	}
	if len(vars) == 0 {
		return func() {}
	}
	w := fe.w
	w.ln("{")
	w.indent++
	for _, v := range vars {
		fe.loadSliceHeader(v)
	}
	return func() {
		for _, v := range vars {
			delete(fe.sliceHdrs, v)
		}
		w.indent--
		w.ln("}")
	}
}

// indexTemp prepares the index i of a load from s for sliceIndex or
// strIndex: an i that cannot be evaluated again (i + 1, a call) is assigned
// to a variable of the function first, so the load stays inline. It returns
// s, the assignment "(t = i, " (to be closed by closeIf), and the index.
// JS evaluates i before s then; s is a plain reference, whose read Go does
// not order against the evaluation of i.
func (fe *funcEmitter) indexTemp(s, i string) (string, string, string) {
	if !fe.inBody || !simpleRef.MatchString(stripMarks(s)) || reusable(stripMarks(i)) {
		return s, "", i
	}
	t := fe.declareName("$i")
	fe.temps = append(fe.temps, t)
	return s, "(" + t + " = " + i + ", ", t
}

func closeIf(set string) string {
	if set == "" {
		return ""
	}
	return ")"
}

// strIndex is the byte s[i] of a string s, inline like sliceIndex.
func strIndex(s, i string) string {
	ss, is := stripMarks(s), stripMarks(i)
	if !reusable(ss) || !reusable(is) {
		return fmt.Sprintf("$rt.strIndex(%s, %s)", s, i)
	}
	cond := fmt.Sprintf("%[2]s >= 0 && %[2]s < %[1]s.length", ss, is)
	if jsLiteral.MatchString(is) {
		cond = fmt.Sprintf("%[2]s < %[1]s.length", ss, is)
	}
	return fmt.Sprintf("(%s ? %[2]s.charCodeAt(%[3]s) : $rt.strIndex(%[2]s, %[3]s))", cond, ss, is)
}

// setSliceIndex is s[i] = v, inline like sliceIndex. JS evaluates the target
// s.$array[...] before v, as Go evaluates s and i before the right-hand side;
// v appears in both branches, so a long v keeps the call of $rt.setIndex.
func setSliceIndex(s, i, v string) string {
	ss, is := stripMarks(s), stripMarks(i)
	if !reusable(ss) || !reusable(is) || len(v) > 120 {
		return fmt.Sprintf("$rt.setIndex(%s, %s, %s)", s, i, v)
	}
	return fmt.Sprintf("(%s ? (%[2]s as any).$array[(%[2]s as any).$offset + %[3]s] = %[4]s : $rt.setIndex(%[2]s, %[3]s, %[4]s))", inBounds(ss, is), ss, is, v)
}

// inBounds is the condition that index i is in range for slice s. A literal
// index is a constant go/types has checked to be non-negative.
func inBounds(s, i string) string {
	if jsLiteral.MatchString(i) {
		return fmt.Sprintf("%[1]s !== null && %[2]s < (%[1]s as any).$length", s, i)
	}
	return fmt.Sprintf("%[1]s !== null && %[2]s >= 0 && %[2]s < (%[1]s as any).$length", s, i)
}

// reusable reports whether the JS expression s (without marks) has the same
// value and no effect when evaluated again: a reference or a literal.
func reusable(s string) bool {
	return simpleRef.MatchString(s) || jsLiteral.MatchString(s)
}

// icallExpr calls method key of interface value recv with the arguments args
// (a list of n values, or a spread), through the runtime's fixed-arity
// icall0 to icall3 where it can.
func icallExpr(recv, key, args string, n int) string {
	switch {
	case args == "":
		return fmt.Sprintf("$rt.icall0(%s, %s)", recv, key)
	case n <= 3 && !strings.HasPrefix(args, "..."):
		return fmt.Sprintf("$rt.icall%d(%s, %s, %s)", n, recv, key, args)
	}
	return fmt.Sprintf("$rt.icall(%s, %s, %s)", recv, key, args)
}

// icall calls method key of interface value recv with the arguments args
// as a method of the interface value, which its box class defines (box.go).
// The property access then has an inline cache at this call site, which
// sees the few dynamic types flowing here, where the shared $rt.icall sees
// every interface method call of the program. A recv that is not a plain reference
// is held in a variable of the function (declared by funcBody, outside one
// in the module's $ir): JS evaluates the callee and recv before the
// arguments, so an interface call among them (or in recv itself) may reuse
// it. A nil interface is a TypeError reading the method, reported as Go's
// nil dereference (see nilChecked).
func (fe *funcEmitter) icall(recv, key, args string) string {
	r, set := stripMarks(recv), ""
	if simpleRef.MatchString(r) {
		r = "(" + r + " as any)"
	} else {
		ir := "$ir"
		if fe.inBody {
			if fe.ir == "" {
				fe.ir = fe.declareName("$ir")
			}
			ir = fe.ir
		} else {
			fe.pe.usesIR = true
		}
		r, set = ir, ir+" = "+recv+", "
	}
	if args != "" {
		args = ", " + args
	}
	return fmt.Sprintf("(%s%s%s(%s%s))", set, r, boxMethodProp(key), r, args)
}

// isStaticFunc reports whether fun names a declared function (never nil).
func isStaticFunc(info *types.Info, fun ast.Expr) bool {
	var id *ast.Ident
	switch f := funcIdent(fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[f]; ok {
			return sel.Kind() == types.MethodExpr // T.M is a function literal
		}
		id = f.Sel
	default:
		return false
	}
	_, ok := info.Uses[id].(*types.Func)
	return ok
}

func funcIdent(fun ast.Expr) ast.Expr {
	switch f := fun.(type) {
	case *ast.IndexExpr:
		return f.X
	case *ast.IndexListExpr:
		return f.X
	}
	return fun
}

func (fe *funcEmitter) awaitIf(call *ast.CallExpr, s string) string {
	if !fe.callBlocks(call) {
		return s
	}
	// An async function resolves to an array of several results, which
	// are moved into the result registers once the call resumes (see
	// multiResult), unless a return statement passes them on as they are.
	untuple := func(s string) string { return s }
	if tt, ok := fe.info.TypeOf(call).(*types.Tuple); ok && tt.Len() > 1 && call != fe.rawCall {
		untuple = func(s string) string { return "$rt.untuple(" + s + ")" }
	}
	if fe.inBody && !fe.pe.prog.CallAlwaysAsync(fe.info, call) && !returnsPointer(fe.info, call) {
		// A function value or interface method of which only some are
		// async: a synchronous one's result is used as it is, without
		// the turn of the event loop an await takes (a Go value is never
		// a JS Promise but for an unsafe.Pointer to one).
		t := fe.declareName("$a")
		fe.temps = append(fe.temps, t)
		return "((" + t + " = " + s + ") instanceof Promise ? " + untuple(fe.await(t)) + " : " + t + ")"
	}
	return "(" + untuple(fe.await(s)) + ")"
}

// returnsPointer reports whether call's only result is an unsafe.Pointer,
// which may be a JS Promise.
func returnsPointer(info *types.Info, call *ast.CallExpr) bool {
	b, ok := info.TypeOf(call).Underlying().(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

// args lowers call arguments: conversions to parameter types, variadic
// packing into a slice, and f(g()) tuple spreading.
func (fe *funcEmitter) args(e *ast.CallExpr, sig *types.Signature) string {
	params := sig.Params()
	n := params.Len()
	paramType := func(i int) types.Type {
		if sig.Variadic() && i >= n-1 {
			if e.Ellipsis.IsValid() {
				return params.At(n - 1).Type()
			}
			return params.At(n - 1).Type().(*types.Slice).Elem()
		}
		return params.At(i).Type()
	}
	var variadicElemTS string
	if sig.Variadic() {
		variadicElemTS = fe.ts(params.At(n - 1).Type().(*types.Slice).Elem())
	}
	var vals []string
	if len(e.Args) == 1 { // f(g()) with g returning several results
		if tt, ok := fe.info.TypeOf(e.Args[0]).(*types.Tuple); ok {
			t := fe.tmp()
			var parts []string
			for i := 0; i < tt.Len(); i++ {
				parts = append(parts, fe.convert(tupleElem(t, i, true), tt.At(i).Type(), paramType(i)))
			}
			if sig.Variadic() {
				fixed := parts[:n-1]
				rest := parts[n-1:]
				parts = append(fixed, "$rt.sliceLit<"+variadicElemTS+">(["+strings.Join(rest, ", ")+"])")
			}
			anys := strings.TrimSuffix(strings.Repeat("any, ", len(parts)), ", ")
			return fmt.Sprintf("...((%s: any): [%s] => [%s])(%s)", t, anys, strings.Join(parts, ", "), fe.expr(e.Args[0]))
		}
	}
	for i, a := range e.Args {
		vals = append(vals, fe.valueOf(a, paramType(i)))
	}
	if sig.Variadic() && !e.Ellipsis.IsValid() {
		fixed := vals
		var rest []string
		if len(vals) >= n-1 {
			fixed, rest = vals[:n-1], vals[n-1:]
		}
		if len(rest) == 0 {
			vals = append(fixed, "null")
		} else {
			vals = append(fixed, "$rt.sliceLit<"+variadicElemTS+">(["+strings.Join(rest, ", ")+"])")
		}
	}
	return strings.Join(vals, ", ")
}

func (fe *funcEmitter) conversion(e *ast.CallExpr, to types.Type) string {
	arg := e.Args[0]
	from := fe.info.TypeOf(arg)
	if s, ok := fe.splitConversion(arg, to); ok {
		return s
	}
	if s, ok := fe.stringBytesRead(arg); ok { // string(b)
		return s
	}
	if s, ok := fe.splitExpr(e); ok {
		return s
	}
	if isUnsafePointer(under(to)) {
		if p, d, ok := fe.pointerArith(arg); ok {
			return fe.mark(e) + "$rt.ptrAdd(" + p + ", " + d + ")"
		}
	}
	s := fe.expr(arg)
	if isNil(from) {
		return "null"
	}
	if isIface(to) {
		if !fe.isFresh(arg) {
			s = fe.pe.copyExpr(s, from, fe.tp)
		}
		return fe.convert(s, from, to)
	}
	tu, fu := under(to), under(from)
	if isUnsafePointer(tu) || isUnsafePointer(fu) {
		return fe.unsafeConversion(e, to, from, s)
	}
	if isTypeParam(to) || isTypeParam(from) {
		// Truncation, wrapping and string conversions depend on the type
		// arguments.
		if tv := fe.info.Types[arg]; tv.Value != nil && !isTypeParam(from) {
			if isNumericConst(tv.Value) && numericTypeSet(to) {
				return fe.constT(tv.Value, to) // representable in every type of to's type set
			}
			from = types.Default(from)
		}
		return fmt.Sprintf("$rt.convertT(%s, %s, %s)", fe.desc(to), fe.desc(from), s)
	}
	if tb, ok := tu.(*types.Basic); ok {
		fb, _ := fu.(*types.Basic)
		switch {
		case tb.Info()&types.IsString != 0:
			if fb != nil && fb.Info()&types.IsInteger != 0 {
				if isBigKind(fb) {
					s = "Number(" + s + ")"
				}
				return "$rt.encodeRune(" + s + ")"
			}
			if sl, ok := fu.(*types.Slice); ok {
				if eb, ok := sl.Elem().Underlying().(*types.Basic); ok && eb.Kind() == types.Int32 {
					return "$rt.runesToString(" + s + ")"
				}
				return "$rt.bytesToString(" + s + ")"
			}
			return s
		case tb.Info()&types.IsInteger != 0:
			ii, _ := intKind(to)
			fi, _ := intKind(from)
			fBig := fb != nil && isBigKind(fb)
			switch {
			case isBigKind(tb) && fb != nil && fb.Info()&types.IsFloat != 0:
				return fmt.Sprintf("$rt.floatToBig(%s, %v)", s, ii.signed)
			case isBigKind(tb) && fBig:
				if fi.signed != ii.signed {
					return wrap(s, ii)
				}
				return s
			case isBigKind(tb):
				if fi.signed == ii.signed && (fi.signed || fi.bits < 64) {
					return "BigInt(" + s + ")"
				}
				return wrap("BigInt("+s+")", ii)
			case fBig:
				if ii.bits < 64 || fi.signed != ii.signed {
					// BigInt.asIntN / asUintN of the narrower width.
					return "Number(" + bigWrap(s, ii) + ")"
				}
				return "Number(" + s + ")"
			}
			if fb != nil && fb.Info()&types.IsFloat != 0 {
				return wrap("$rt.trunc("+s+")", ii)
			}
			if fi.bits > ii.bits || fi.signed != ii.signed || ii.bits < 64 {
				return wrap(s, ii)
			}
			return s
		case tb.Kind() == types.Complex64:
			if fb != nil && fb.Kind() == types.Complex64 {
				return s
			}
			return "$rt.c64(" + s + ")"
		case tb.Info()&types.IsComplex != 0:
			return s
		case tb.Kind() == types.Float32:
			if fb != nil && isBigKind(fb) {
				s = "Number(" + s + ")"
			}
			return "$rt.fround(" + s + ")"
		case tb.Info()&types.IsFloat != 0:
			if fb != nil && isBigKind(fb) {
				return "Number(" + s + ")"
			}
			return s
		}
	}
	if sl, ok := tu.(*types.Slice); ok {
		if fb, ok := fu.(*types.Basic); ok && fb.Info()&types.IsString != 0 {
			if eb, ok := sl.Elem().Underlying().(*types.Basic); ok && eb.Kind() == types.Int32 {
				return "$rt.stringToRunes(" + s + ")"
			}
			return "$rt.stringToBytes(" + s + ")"
		}
	}
	if _, ok := tu.(*types.Array); ok {
		if _, ok := fu.(*types.Slice); ok {
			n := tu.(*types.Array).Len()
			return fmt.Sprintf("$rt.sliceToArray(%s, %d)", s, n)
		}
	}
	if p, ok := tu.(*types.Pointer); ok {
		if _, ok := under(p.Elem()).(*types.Array); ok {
			if _, ok := fu.(*types.Slice); ok {
				return fmt.Sprintf("$rt.sliceToArrayPtr(%s, %d)", s, under(p.Elem()).(*types.Array).Len())
			}
		}
	}
	if st, ok := tu.(*types.Struct); ok && !types.Identical(to, from) {
		// Another struct type: build an instance of its class so the value
		// carries the destination type's representation.
		x := fe.tmp()
		var fields []string
		for i := 0; i < st.NumFields(); i++ {
			fields = append(fields, fe.pe.copyExpr(x+"."+fieldProp(st, i), st.Field(i).Type(), fe.tp))
		}
		return fmt.Sprintf("((%s: any) => new %s(%s))(%s)", x, fe.pe.structClass(to), strings.Join(fields, ", "), s)
	}
	return s // identical underlying types: representation unchanged
}

func (fe *funcEmitter) builtin(e *ast.CallExpr, name string) string {
	if len(e.Args) == 1 && (name == "complex" || name == "append" || name == "copy" || name == "delete") {
		if tt, ok := fe.info.TypeOf(e.Args[0]).(*types.Tuple); ok {
			// complex(f()), append(f()), copy(f()), delete(f()): the results of f are
			// the arguments.
			t := fe.tmp()
			call := *e
			call.Args = make([]ast.Expr, tt.Len())
			for i := range call.Args {
				id := &ast.Ident{NamePos: e.Args[0].Pos(), Name: "_"}
				fe.info.Types[id] = types.TypeAndValue{Type: tt.At(i).Type()}
				fe.override[id] = tupleElem(t, i, true)
				call.Args[i] = id
			}
			fe.info.Types[&call] = fe.info.Types[e]
			return fmt.Sprintf("((%s: any) => %s)(%s)", t, fe.builtin(&call, name), fe.expr(e.Args[0]))
		}
	}
	arg := func(i int) string { return fe.expr(e.Args[i]) }
	m := fe.mark(e)
	switch name {
	case "len", "cap":
		if s, ok := fe.stringBytesRead(e.Args[0]); ok && name == "len" {
			return s + ".length"
		}
		t := fe.info.TypeOf(e.Args[0])
		if p, ok := under(t).(*types.Pointer); ok {
			t = p.Elem()
		}
		switch u := under(t).(type) {
		case *types.Basic:
			return arg(0) + ".length"
		case *types.Slice:
			if h, ok := fe.sliceHdr(e.Args[0]); ok && name == "len" {
				return h.length
			}
			if a := arg(0); reusable(stripMarks(a)) {
				prop := "$length"
				if name == "cap" {
					prop = "$capacity"
				}
				return fmt.Sprintf("(%[1]s === null ? 0 : (%[1]s as any).%[2]s)", stripMarks(a), prop)
			}
			return "$rt." + name + "(" + arg(0) + ")"
		case *types.Array:
			if tv := fe.info.Types[e]; tv.Value == nil {
				// Not constant: the operand has calls or receives to evaluate.
				return fmt.Sprintf("(%s, %d)", arg(0), u.Len())
			}
			return fmt.Sprint(u.Len())
		case *types.Map:
			return "$rt.mapLen(" + arg(0) + ")"
		case *types.Interface: // type parameter without core type
			return "$rt." + name + "Any(" + arg(0) + ")"
		case *types.Chan:
			if name == "len" {
				return "$rt.chanLen(" + arg(0) + ")"
			}
			return "$rt.chanCap(" + arg(0) + ")"
		}
	case "append":
		elem := under(fe.info.TypeOf(e)).(*types.Slice).Elem()
		et := fe.elemTypeArg(elem)
		if e.Ellipsis.IsValid() {
			src := arg(1)
			if b, ok := under(fe.info.TypeOf(e.Args[1])).(*types.Basic); ok && b.Info()&types.IsString != 0 {
				return fmt.Sprintf("%s$rt.appendString(%s, %s)", m, arg(0), src)
			} else if isTypeParam(fe.info.TypeOf(e.Args[1])) {
				src += " as any" // ~string | ~[]byte: toArray handles both
			} else if et == "" {
				return fmt.Sprintf("%s$rt.appendSlice<%s>(%s, %s, %s)", m, fe.ts(elem), arg(0), src, fe.zeroFn(elem))
			}
			return fmt.Sprintf("%s$rt.append<%s>(%s, $rt.toArray(%s), %s%s)", m, fe.ts(elem), arg(0), src, fe.zeroFn(elem), et)
		}
		var vals []string
		for _, a := range e.Args[1:] {
			vals = append(vals, fe.valueOf(a, elem))
		}
		if len(vals) == 1 && et == "" {
			return fmt.Sprintf("%s$rt.append1<%s>(%s, %s, %s)", m, fe.ts(elem), arg(0), vals[0], fe.zeroFn(elem))
		}
		if len(vals) == 1 { // valueOf made the value for the call alone
			return fmt.Sprintf("%s$rt.appendNew1<%s>(%s, %s, %s%s)", m, fe.ts(elem), arg(0), vals[0], fe.zeroFn(elem), et)
		}
		return fmt.Sprintf("%s$rt.append<%s>(%s, [%s], %s%s)", m, fe.ts(elem), arg(0), strings.Join(vals, ", "), fe.zeroFn(elem), et)
	case "copy":
		et := ""
		if sl, ok := under(fe.info.TypeOf(e.Args[0])).(*types.Slice); ok {
			et = fe.elemTypeArg(sl.Elem())
		}
		return fmt.Sprintf("%s$rt.sliceCopy(%s, %s%s)", m, arg(0), arg(1), et)
	case "delete":
		mt := under(fe.info.TypeOf(e.Args[0])).(*types.Map)
		return fmt.Sprintf("%s$rt.mapDelete(%s, %s)", m, arg(0), fe.valueOf(e.Args[1], mt.Key()))
	case "make":
		t := fe.info.TypeOf(e.Args[0])
		switch u := under(t).(type) {
		case *types.Slice:
			l, c := fe.intNumber(e.Args[1]), "undefined"
			if len(e.Args) > 2 {
				c = fe.intNumber(e.Args[2])
			}
			zero := fe.zeroFn(u.Elem())
			if fe.pe.byteBoolMakes[e] {
				zero = "($rt.zeroByte as any)" // see byteBools
			}
			return fmt.Sprintf("%s$rt.makeSlice(%s, %s, %s)", m, l, c, zero)
		case *types.Map:
			if len(e.Args) > 1 { // the size hint is evaluated, then unused
				return fmt.Sprintf("%s$rt.makeMap(%s, %s)", m, fe.desc(u.Key()), arg(1))
			}
			return fmt.Sprintf("%s$rt.makeMap(%s)", m, fe.desc(u.Key()))
		case *types.Chan:
			c := "0"
			if len(e.Args) > 1 {
				c = fe.intNumber(e.Args[1])
			}
			size := ""
			if sizes := fe.pe.pkg.TypesSizes; sizes != nil && !isGenericType(u.Elem()) {
				if n := sizes.Sizeof(u.Elem()); n > 1 {
					size = fmt.Sprintf(", %d", n) // for the size limit
				}
			}
			return fmt.Sprintf("%s$rt.makeChan(%s, %s%s)", m, c, fe.zeroFn(u.Elem()), size)
		}
	case "new":
		t := fe.info.TypeOf(e).(*types.Pointer).Elem()
		if tv := fe.info.Types[e.Args[0]]; !tv.IsType() {
			// new(expr) (Go 1.26): a new variable initialised to expr.
			v := fe.valueOf(e.Args[0], t)
			switch {
			case isTypeParam(t):
				return "$rt.newPtrOf(" + fe.desc(t) + ", " + v + ")"
			case isAggregate(t):
				return v // a fresh copy: the object is the pointer
			}
			return "$rt.cell<" + fe.ts(t) + ">(" + v + ")"
		}
		if isTypeParam(t) {
			return "$rt.newPtr(" + fe.desc(t) + ")"
		}
		if isAggregate(t) {
			return fe.zero(t)
		}
		return "$rt.cell<" + fe.ts(t) + ">(" + fe.zero(t) + ")"
	case "panic":
		return fmt.Sprintf("%s$rt.panic(%s)", m, fe.valueOf(e.Args[0], types.Universe.Lookup("any").Type()))
	case "recover":
		return m + "$rt.recover($rf)"
	case "print", "println":
		if len(e.Args) == 1 {
			if tt, ok := fe.info.TypeOf(e.Args[0]).(*types.Tuple); ok { // println(f())
				t := fe.tmp()
				var parts []string
				for i := 0; i < tt.Len(); i++ {
					parts = append(parts, fe.printArg(tt.At(i).Type(), tupleElem(t, i, true)))
				}
				return fmt.Sprintf("%s$rt.%s(...((%s: any) => [%s])(%s))", m, name, t, strings.Join(parts, ", "), arg(0))
			}
		}
		var vals []string
		for i, a := range e.Args {
			if tv := fe.info.Types[a]; tv.Value != nil && tv.Value.Kind() == constant.Int && isIntegerType(tv.Type) {
				// Exact even where int is a JS number (beyond 2^53).
				vals = append(vals, jsString(tv.Value.ExactString()))
				continue
			}
			vals = append(vals, fe.printArg(fe.info.TypeOf(a), arg(i)))
		}
		return m + "$rt." + name + "(" + strings.Join(vals, ", ") + ")"
	case "close":
		return m + "$rt.close(" + arg(0) + ")"
	case "clear":
		t := fe.info.TypeOf(e.Args[0])
		if sl, ok := under(t).(*types.Slice); ok {
			return fmt.Sprintf("%s$rt.sliceClear(%s, %s%s)", m, arg(0), fe.zeroFn(sl.Elem()), fe.elemTypeArg(sl.Elem()))
		}
		return m + "$rt.mapClear(" + arg(0) + ")"
	case "real":
		return "(" + arg(0) + ").re"
	case "imag":
		return "(" + arg(0) + ").im"
	case "complex":
		return "$rt.complex(" + arg(0) + ", " + arg(1) + ")"
	case "min", "max":
		var vals []string
		for i := range e.Args {
			vals = append(vals, arg(i))
		}
		return "$rt." + name + "(" + strings.Join(vals, ", ") + ")"
	}
	fe.errorf(e.Pos(), "builtin %s is not supported yet", name)
	return "undefined"
}

// isGenericType reports whether t is or mentions a type parameter, or is an
// instance of a generic type.
func isGenericType(t types.Type) bool {
	if n, ok := types.Unalias(t).(*types.Named); ok && n.TypeArgs().Len() > 0 {
		return true
	}
	return hasTypeParam(t)
}

// elemTypeArg is the trailing element-type argument of $rt.append and
// $rt.sliceCopy, passed when elements may be aggregates that must be copied.
func (fe *funcEmitter) elemTypeArg(elem types.Type) string {
	if _, isTP := types.Unalias(elem).(*types.TypeParam); isTP || isAggregate(elem) {
		return ", " + fe.desc(elem)
	}
	return ""
}

// ---- unsafe ----
//
// An unsafe.Pointer holds the pointer object itself (see runtime/src/ptr.ts):
// converting a pointer to unsafe.Pointer and back to the same pointer type is
// the identity. There is no address space, so pointer arithmetic and
// conversions from uintptr are diagnosed; uintptr(p) is a stand-in address
// ($rt.addressOf). Pointers keep their provenance instead (runtime/src/
// unsafe.ts): unsafe.String and unsafe.Slice work on pointers into a slice,
// array or string (&x[i], unsafe.SliceData, unsafe.StringData), and
// reinterpreting memory works between layouts goesm can view as each other
// (see reinterpretable).

func isUnsafePointer(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

func (fe *funcEmitter) unsafeConversion(e *ast.CallExpr, to, from types.Type, s string) string {
	tu, fu := under(to), under(from)
	if isUnsafePointer(tu) && isUnsafePointer(fu) {
		return s
	}
	if isUnsafePointer(tu) {
		if _, ok := fu.(*types.Pointer); ok {
			return s
		}
		if b, ok := fu.(*types.Basic); ok && b.Kind() == types.Uintptr {
			return "$rt.fromAddress(" + s + ")" // the pointer whose stand-in address it is
		}
		fe.errorf(e.Pos(), "conversion from %s to unsafe.Pointer is not supported (goesm has no address space)", from)
		return s
	}
	if b, ok := tu.(*types.Basic); ok && b.Kind() == types.Uintptr {
		return "$rt.addressOf(" + s + ")" // a stand-in address, for printing and hashing
	}
	tp, ok := tu.(*types.Pointer)
	if !ok {
		fe.errorf(e.Pos(), "conversion from unsafe.Pointer to %s is not supported (goesm has no address space)", to)
		return s
	}
	// (*T)(unsafe.Pointer(p)) with p of type *U reinterprets U's memory as T.
	if inner, ok := unparen(e.Args[0]).(*ast.CallExpr); ok && len(inner.Args) == 1 {
		if tv, ok := fe.info.Types[inner.Fun]; ok && tv.IsType() && isUnsafePointer(under(tv.Type)) {
			if up, ok := under(fe.info.TypeOf(inner.Args[0])).(*types.Pointer); ok && !types.Identical(under(up.Elem()), under(tp.Elem())) {
				if pointerShaped(up.Elem()) && pointerShaped(tp.Elem()) {
					return s // a pointer variable read as another pointer type: the same reference
				}
				if reinterpretable(up.Elem(), tp.Elem()) {
					return fmt.Sprintf("$rt.reinterpret(%s, %s, %s)", s, fe.desc(up.Elem()), fe.desc(tp.Elem()))
				}
				if fe.pe.std || !startsWith(up.Elem(), tp.Elem()) {
					fe.errorf(e.Pos(), "reinterpreting %s as %s through unsafe.Pointer is not supported", up, to)
					return s
				}
			}
		}
	}
	if fe.pe.std {
		return s // the standard library converts back what it converted to unsafe.Pointer
	}
	// The pointer may point into the middle of an aggregate (unsafe pointer
	// arithmetic) or be a struct read as its first field.
	return fmt.Sprintf("$rt.ptrAt(%s, %s)", s, fe.desc(tp.Elem()))
}

// startsWith reports whether a value of type t begins with a variable of type
// first: a pointer to t is then also a pointer to it (ptrAt finds it).
func startsWith(t, first types.Type) bool {
	for {
		switch u := under(t).(type) {
		case *types.Struct:
			if u.NumFields() == 0 {
				return false
			}
			t = u.Field(0).Type()
		case *types.Array:
			if u.Len() == 0 {
				return false
			}
			t = u.Elem()
		default:
			return false
		}
		if types.Identical(under(t), under(first)) {
			return true
		}
	}
}

// pointerArith matches unsafe.Pointer(uintptr(p) + d) and uintptr(p) - d,
// the pointer arithmetic unsafe.Pointer's rules allow, and returns p and the
// signed offset.
func (fe *funcEmitter) pointerArith(arg ast.Expr) (p, d string, ok bool) {
	be, ok := unparen(arg).(*ast.BinaryExpr)
	if !ok || (be.Op != token.ADD && be.Op != token.SUB) {
		return "", "", false
	}
	c, ok := unparen(be.X).(*ast.CallExpr)
	if !ok || len(c.Args) != 1 {
		return "", "", false
	}
	tv, ok := fe.info.Types[c.Fun]
	if !ok || !tv.IsType() {
		return "", "", false
	}
	if b, ok := under(tv.Type).(*types.Basic); !ok || b.Kind() != types.Uintptr {
		return "", "", false
	}
	switch under(fe.info.TypeOf(c.Args[0])).(type) {
	case *types.Pointer:
	case *types.Basic:
		if !isUnsafePointer(under(fe.info.TypeOf(c.Args[0]))) {
			return "", "", false
		}
	default:
		return "", "", false
	}
	p, d = fe.expr(c.Args[0]), fe.expr(be.Y)
	if be.Op == token.SUB {
		d = "-(" + d + ")"
	}
	return p, d, true
}

// reinterpretable reports whether goesm can view memory of type from as type
// to (see runtime/src/unsafe.ts): structs of the same layout, and the header
// structs that mirror a string ({data, len}), a slice ({data, len, cap}) or
// an interface ({type, data}), in both directions for strings and slices,
// and a []byte read as a string or the other way round.
func reinterpretable(from, to types.Type) bool {
	fu, tu := under(from), under(to)
	ptrLike := pointerShaped
	isInt := func(t types.Type) bool {
		b, ok := under(t).(*types.Basic)
		return ok && b.Info()&types.IsInteger != 0 && b.Kind() != types.Uintptr
	}
	isString := func(t types.Type) bool {
		b, ok := under(t).(*types.Basic)
		return ok && b.Info()&types.IsString != 0
	}
	// header matches the layout of a struct's non-zero-size fields.
	header := func(t types.Type, want ...func(types.Type) bool) bool {
		st, ok := under(t).(*types.Struct)
		if !ok {
			return false
		}
		var ws []types.Type
		for i := 0; i < st.NumFields(); i++ {
			if f := st.Field(i).Type(); !zeroSized(f) {
				ws = append(ws, f)
			}
		}
		if len(ws) != len(want) {
			return false
		}
		for i, w := range ws {
			if !want[i](w) {
				return false
			}
		}
		return true
	}
	switch fu := fu.(type) {
	case *types.Struct:
		switch tu := tu.(type) {
		case *types.Struct:
			var fw, tw []types.Type
			for i := 0; i < fu.NumFields(); i++ {
				if f := fu.Field(i).Type(); !zeroSized(f) {
					fw = append(fw, f)
				}
			}
			for i := 0; i < tu.NumFields(); i++ {
				if f := tu.Field(i).Type(); !zeroSized(f) {
					tw = append(tw, f)
				}
			}
			if len(fw) != len(tw) {
				return false
			}
			for i := range fw {
				if !types.Identical(under(fw[i]), under(tw[i])) && !(ptrLike(fw[i]) && ptrLike(tw[i])) {
					return false
				}
			}
			return true
		case *types.Slice, *types.Basic:
			if b, ok := tu.(*types.Basic); ok && b.Info()&types.IsString == 0 {
				return false
			}
			return header(from, isString, isInt) || header(from, isString, isInt, isInt) ||
				header(from, ptrLike, isInt) || header(from, ptrLike, isInt, isInt)
		}
	case *types.Interface:
		if a, ok := tu.(*types.Array); ok {
			return fu.Empty() && a.Len() == 2 && ptrLike(a.Elem())
		}
		return fu.Empty() && header(to, ptrLike, ptrLike)
	case *types.Basic:
		return fu.Info()&types.IsString != 0 && (header(to, ptrLike, isInt) || isByteSlice(tu))
	case *types.Slice:
		return header(to, ptrLike, isInt, isInt) || isByteSlice(fu) && isString(tu)
	}
	return false
}

// isByteSlice reports whether t is a slice of bytes.
func isByteSlice(t types.Type) bool {
	s, ok := under(t).(*types.Slice)
	if !ok {
		return false
	}
	b, ok := under(s.Elem()).(*types.Basic)
	return ok && b.Kind() == types.Byte
}

// pointerShaped reports whether a value of type t is one machine pointer.
func pointerShaped(t types.Type) bool {
	switch u := under(t).(type) {
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		return true
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	}
	return false
}

func zeroSized(t types.Type) bool {
	switch u := under(t).(type) {
	case *types.Array:
		return u.Len() == 0 || zeroSized(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if !zeroSized(u.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

// unsafeElem matches a pointer operand that addresses an element of a slice
// or array: &x[i] or unsafe.SliceData(x). It returns the slice or array
// expression and the index; checked reports whether x[i] must be in range.
func (fe *funcEmitter) unsafeElem(ptr ast.Expr) (base, idx string, checked, ok bool) {
	switch p := unparen(ptr).(type) {
	case *ast.UnaryExpr:
		if ix, isIdx := unparen(p.X).(*ast.IndexExpr); p.Op == token.AND && isIdx {
			switch under(fe.info.TypeOf(ix.X)).(type) {
			case *types.Slice, *types.Array, *types.Pointer:
				return fe.expr(ix.X), fe.intNumber(ix.Index), true, true
			}
		}
	case *ast.CallExpr:
		if se, isSel := unparen(p.Fun).(*ast.SelectorExpr); isSel {
			if b, isB := fe.info.Uses[se.Sel].(*types.Builtin); isB && b.Name() == "SliceData" {
				return fe.expr(p.Args[0]), "0", false, true
			}
		}
	}
	return "", "", false, false
}

func (fe *funcEmitter) unsafeCall(e *ast.CallExpr, name string) string {
	m := fe.mark(e)
	switch name {
	case "String":
		if base, idx, checked, ok := fe.unsafeElem(e.Args[0]); ok {
			return fmt.Sprintf("%s$rt.bytesToString($rt.unsafeSlice(%s, %s, %s, %v))", m, base, idx, fe.intNumber(e.Args[1]), checked)
		}
		// Any other pointer: follow its provenance at run time.
		return fmt.Sprintf("%s$rt.unsafeStringFrom(%s, %s)", m, fe.expr(e.Args[0]), fe.intNumber(e.Args[1]))
	case "Slice":
		if base, idx, checked, ok := fe.unsafeElem(e.Args[0]); ok {
			return fmt.Sprintf("%s$rt.unsafeSlice(%s, %s, %s, %v)", m, base, idx, fe.intNumber(e.Args[1]), checked)
		}
		if c, ok := unparen(e.Args[0]).(*ast.CallExpr); ok {
			if se, ok := unparen(c.Fun).(*ast.SelectorExpr); ok {
				if b, ok := fe.info.Uses[se.Sel].(*types.Builtin); ok && b.Name() == "StringData" {
					// The bytes of a string are immutable, so a copy is
					// indistinguishable from an alias.
					return fmt.Sprintf("%s$rt.stringToBytes($rt.substr(%s, 0, %s))", m, fe.expr(c.Args[0]), fe.intNumber(e.Args[1]))
				}
			}
		}
		return fmt.Sprintf("%s$rt.unsafeSliceFrom(%s, %s)", m, fe.expr(e.Args[0]), fe.intNumber(e.Args[1]))
	case "StringData":
		return fmt.Sprintf("%s$rt.stringData(%s)", m, fe.expr(e.Args[0]))
	case "Sizeof", "Alignof": // not constant: the operand's type involves a type parameter
		return fmt.Sprintf("%s$rt.%sOf(%s)", m, strings.ToLower(name[:len(name)-2]), fe.desc(fe.info.TypeOf(e.Args[0])))
	case "SliceData":
		return fmt.Sprintf("%s$rt.sliceData(%s, %v)", m, fe.expr(e.Args[0]), isAggregate(under(fe.info.TypeOf(e.Args[0])).(*types.Slice).Elem()))
	case "Add":
		return fmt.Sprintf("%s$rt.ptrAdd(%s, %s)", m, fe.expr(e.Args[0]), fe.intNumber(e.Args[1]))
	}
	fe.errorf(e.Pos(), "unsafe.%s is not supported in this form (goesm has no address space)", name)
	return "undefined"
}

// printArg formats an operand of the print builtins as the Go runtime does
// where JS's String() differs.
func (fe *funcEmitter) printArg(t types.Type, v string) string {
	if _, ok := types.Unalias(t).(*types.TypeParam); ok {
		return "$rt.printTyped(" + fe.desc(t) + ", " + v + ")" // the format depends on the type argument
	}
	t = types.Default(t)
	if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return `"nil"`
	}
	switch u := under(t).(type) {
	case *types.Basic:
		switch {
		case u.Kind() == types.Float32:
			return "$rt.printFloat(" + v + ", 32)"
		case u.Info()&types.IsFloat != 0:
			return "$rt.printFloat(" + v + ")"
		case u.Kind() == types.Complex64:
			return "$rt.printComplex(" + v + ", 32)"
		case u.Info()&types.IsComplex != 0:
			return "$rt.printComplex(" + v + ")"
		case u.Kind() == types.UnsafePointer:
			return "$rt.printPointer(" + v + ")"
		}
	case *types.Interface:
		return "$rt.printIface(" + v + ")"
	case *types.Slice:
		return "$rt.printSlice(" + v + ")"
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature:
		return "$rt.printPointer(" + v + ")"
	}
	return v
}

func isIntegerType(t types.Type) bool {
	b, ok := under(t).(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}
