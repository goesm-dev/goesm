package lower

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
)

// expr lowers an expression to a JS expression string. The result denotes
// the Go value without copying it; contexts that need Go copy semantics
// use valueOf.
func (fe *funcEmitter) expr(e ast.Expr) string {
	if s, ok := fe.override[e]; ok {
		return s
	}
	if tv, ok := fe.info.Types[e]; ok && tv.Value != nil {
		if tv.Value.Kind() == constant.Complex {
			fe.errorf(e.Pos(), "complex numbers are not supported yet")
			return "0"
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
		return fe.mark(e) + "$rt.deref(" + p + ").v"
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
	return fmt.Sprintf("((...a: any[]) => %s(%s, ...a))", fn, strings.Join(fe.typeArgList(targs), ", "))
}

func (fe *funcEmitter) funcInstance(e ast.Expr, x ast.Expr) string {
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
		return fmt.Sprintf("$rt.box(%s, %s)", fe.desc(from), s)
	}
	return s
}

func (fe *funcEmitter) convertCopy(s string, from, to types.Type) string {
	return fe.convert(fe.pe.copyExpr(s, from, fe.tp), from, to)
}

// fieldBase resolves a field selector (including promoted fields) to the JS
// object holding the field and the property name.
func (fe *funcEmitter) fieldBase(e *ast.SelectorExpr) (string, string) {
	sel := fe.info.Selections[e]
	obj := fe.expr(e.X)
	t := fe.info.TypeOf(e.X)
	path := sel.Index()
	for i, idx := range path {
		base, _ := derefType(t)
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
		obj, prop := fe.fieldBase(e)
		return fe.mark(e) + obj + "." + prop
	case types.MethodVal:
		fn, recv, iface := fe.methodTarget(e, sel)
		r := fe.tmp()
		if iface {
			return fmt.Sprintf("((%s: any) => (...a: any[]) => $rt.icall(%s, %s, ...a))(%s)", r, r, jsString(methodKey(sel.Obj().(*types.Func))), recv)
		}
		// Method value: the receiver is evaluated (and copied) now.
		if fnT := sel.Obj().(*types.Func); !isPtrRecv(fnT) {
			recv = fe.pe.copyExpr(recv, fnT.Signature().Recv().Type(), fe.tp)
		}
		return fmt.Sprintf("((%s: any) => (...a: any[]) => %s%s, ...a))(%s)", r, fn, r, recv)
	case types.MethodExpr:
		fn := sel.Obj().(*types.Func)
		recvT := sel.Recv()
		if isIface(recvT) {
			return fmt.Sprintf("((r: any, ...a: any[]) => $rt.icall(r, %s, ...a))", jsString(methodKey(fn)))
		}
		base, havePtr := derefType(recvT)
		recv := "r"
		if !isPtrRecv(fn) && havePtr {
			recv = fmt.Sprintf("$rt.derefMethod(r, %s)", jsString(panicwrapMsg(fn, base)))
			if !isAggregate(base) {
				recv += ".v"
			}
		}
		return fmt.Sprintf("((r: any, ...a: any[]) => %s(%s%s, ...a))", fe.pe.methodFuncName(fn), fe.pe.recvTypeArgs(base, fe.tp), recv)
	}
	return "undefined"
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
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP {
			fe.errorf(e.Pos(), "taking the address of a type-parameter-typed variable is not supported yet")
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
		if isAggregate(t) {
			return obj + "." + prop
		}
		return fmt.Sprintf("%s$rt.fieldPtr(%s, %s)", fe.mark(x), obj, jsString(prop))
	case *ast.IndexExpr:
		xt := fe.info.TypeOf(x.X)
		if _, ok := xt.Underlying().(*types.Slice); ok {
			if isAggregate(t) {
				return fmt.Sprintf("%s$rt.index(%s, %s)", fe.mark(x), fe.expr(x.X), fe.expr(x.Index))
			}
			return fmt.Sprintf("%s$rt.sliceElemPtr(%s, %s)", fe.mark(x), fe.expr(x.X), fe.expr(x.Index))
		}
		if isAggregate(t) {
			return fmt.Sprintf("%s[%s]", fe.expr(x.X), fe.arrayIndex(x))
		}
		return fmt.Sprintf("%s$rt.arrayElemPtr(%s, %s)", fe.mark(x), fe.expr(x.X), fe.expr(x.Index))
	case *ast.StarExpr:
		return fe.expr(x.X)
	case *ast.CompositeLit:
		if isAggregate(t) {
			return fe.compositeLit(x)
		}
		return "$rt.cell(" + fe.compositeLit(x) + ")"
	}
	fe.errorf(e.Pos(), "unsupported address-of operand %T", e)
	return "undefined"
}

func (fe *funcEmitter) arrayIndex(x *ast.IndexExpr) string {
	if tv, ok := fe.info.Types[x.Index]; ok && tv.Value != nil {
		return fe.expr(x.Index) // constant indices are bounds-checked by go/types
	}
	xt := fe.info.TypeOf(x.X)
	base, _ := derefType(xt)
	n := base.Underlying().(*types.Array).Len()
	return fmt.Sprintf("$rt.arrayIndex(%d, %s)", n, fe.expr(x.Index))
}

func (fe *funcEmitter) index(e *ast.IndexExpr) string {
	if _, ok := fe.info.Instances[identOf(e.X)]; ok {
		return fe.funcInstance(e, e.X)
	}
	xt := fe.info.TypeOf(e.X)
	m := fe.mark(e)
	switch u := under(xt).(type) {
	case *types.Basic:
		return fmt.Sprintf("%s$rt.strIndex(%s, %s)", m, fe.expr(e.X), fe.expr(e.Index))
	case *types.Slice:
		return fmt.Sprintf("%s$rt.index(%s, %s)", m, fe.expr(e.X), fe.expr(e.Index))
	case *types.Map:
		return fmt.Sprintf("%s$rt.mapGet(%s, %s, %s)", m, fe.expr(e.X), fe.valueOf(e.Index, u.Key()), fe.zeroFn(u.Elem()))
	case *types.Array:
		return fmt.Sprintf("%s%s[%s]", m, fe.expr(e.X), fe.arrayIndex(e))
	case *types.Pointer: // *array: the array object, nil-checked
		return fmt.Sprintf("%s$rt.deref(%s)[%s]", m, fe.expr(e.X), fe.arrayIndex(e))
	case *types.Interface:
		// Type parameter without a core type, e.g. ~string | ~[]byte: the
		// representation is chosen at run time.
		return fmt.Sprintf("%s$rt.indexAny(%s, %s)", m, fe.expr(e.X), fe.expr(e.Index))
	}
	fe.errorf(e.Pos(), "unsupported index expression on %s", xt)
	return "undefined"
}

func (fe *funcEmitter) sliceExpr(e *ast.SliceExpr) string {
	opt := func(x ast.Expr) string {
		if x == nil {
			return "undefined"
		}
		return fe.expr(x)
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
	t := fe.info.TypeOf(e)
	if p, ok := t.Underlying().(*types.Pointer); ok { // elided &T in nested literals
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
		for i, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				name := kv.Key.(*ast.Ident).Name
				for j := 0; j < u.NumFields(); j++ {
					if u.Field(j).Name() == name {
						elems = append(elems, elemVal{j, fe.valueOf(kv.Value, u.Field(j).Type())})
					}
				}
				continue
			}
			elems = append(elems, elemVal{i, fe.valueOf(el, u.Field(i).Type())})
		}
		pre := fe.spillOutOfOrder(elems)
		for _, ev := range elems {
			vals[ev.slot] = ev.val
		}
		return wrapPre(pre, fmt.Sprintf("%snew %s(%s)", m, fe.pe.structClass(t), strings.Join(vals, ", ")))
	case *types.Array:
		pre, vals := fe.indexedElems(e, u.Elem(), int(u.Len()))
		return wrapPre(pre, m+"["+strings.Join(vals, ", ")+"]")
	case *types.Slice:
		pre, vals := fe.indexedElems(e, u.Elem(), -1)
		return wrapPre(pre, m+"$rt.sliceLit(["+strings.Join(vals, ", ")+"])")
	case *types.Map:
		var kvs []string
		for _, el := range e.Elts {
			kv := el.(*ast.KeyValueExpr)
			kvs = append(kvs, fmt.Sprintf("[%s, %s]", fe.valueOf(kv.Key, u.Key()), fe.valueOf(kv.Value, u.Elem())))
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
}

func intKind(t types.Type) (intInfo, bool) {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return intInfo{}, false
	}
	switch b.Kind() {
	case types.Int8:
		return intInfo{8, true}, true
	case types.Int16:
		return intInfo{16, true}, true
	case types.Int32, types.UntypedRune:
		return intInfo{32, true}, true
	case types.Int, types.Int64, types.UntypedInt:
		return intInfo{64, true}, true
	case types.Uint8:
		return intInfo{8, false}, true
	case types.Uint16:
		return intInfo{16, false}, true
	case types.Uint32:
		return intInfo{32, false}, true
	case types.Uint, types.Uint64, types.Uintptr:
		return intInfo{64, false}, true
	}
	return intInfo{}, false
}

// wrap truncates an integer result to its Go width. 64-bit integers are JS
// numbers in the PoC and are not wrapped (documented gap).
func wrap(s string, ii intInfo) string {
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
	return s
}

func isTypeParam(t types.Type) bool {
	_, ok := types.Unalias(t).(*types.TypeParam)
	return ok
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
			return wrap("$rt.div("+a+", "+b+")", ii)
		case token.REM:
			return wrap("$rt.mod("+a+", "+b+")", ii)
		case token.AND, token.OR, token.XOR, token.AND_NOT:
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
	s := "(" + a + " " + op.String() + " " + b + ")"
	if isFloat32(t) {
		return "$rt.fround" + s
	}
	return s
}

func (fe *funcEmitter) shift(op token.Token, a, n string, t types.Type) string {
	if isTypeParam(t) {
		return fmt.Sprintf("$rt.shiftT(%s, %v, %s, %s)", fe.desc(t), op == token.SHL, a, n)
	}
	ii, _ := intKind(t)
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
		return fmt.Sprintf("$rt.equal(%s, %s, %s)", fe.desc(at), a, b)
	}
	if isAggregate(at) {
		return fmt.Sprintf("$rt.equal(%s, %s, %s)", fe.desc(at), a, b)
	}
	return "(" + a + " === " + b + ")"
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
	xt, yt := fe.info.TypeOf(e.X), fe.info.TypeOf(e.Y)
	switch e.Op {
	case token.LAND, token.LOR:
		return "(" + fe.expr(e.X) + " " + e.Op.String() + " " + fe.expr(e.Y) + ")"
	case token.EQL:
		return fe.eqExpr(fe.expr(e.X), xt, fe.expr(e.Y), yt)
	case token.NEQ:
		return "!" + "(" + fe.eqExpr(fe.expr(e.X), xt, fe.expr(e.Y), yt) + ")"
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
		return "(" + fe.expr(e.X) + " " + e.Op.String() + " " + fe.expr(e.Y) + ")"
	case token.SHL, token.SHR:
		return fe.mark(e) + fe.shift(e.Op, fe.expr(e.X), fe.expr(e.Y), fe.info.TypeOf(e))
	}
	return fe.mark(e) + fe.arith(e.Op, fe.expr(e.X), fe.expr(e.Y), fe.info.TypeOf(e))
}

func (fe *funcEmitter) unary(e *ast.UnaryExpr) string {
	t := fe.info.TypeOf(e)
	switch e.Op {
	case token.AND:
		return fe.addrOf(e.X)
	case token.ARROW:
		return fmt.Sprintf("%s(await $rt.recv(%s))[0]", fe.mark(e), fe.expr(e.X))
	case token.NOT:
		return "!" + fe.expr(e.X)
	case token.ADD:
		return fe.expr(e.X)
	case token.SUB:
		if isTypeParam(t) {
			return fmt.Sprintf("$rt.negT(%s, %s)", fe.desc(t), fe.expr(e.X))
		}
		if ii, ok := intKind(t); ok {
			return wrap("-"+fe.expr(e.X), ii)
		}
		if isFloat32(t) {
			return "$rt.fround(-" + fe.expr(e.X) + ")"
		}
		return "(-" + fe.expr(e.X) + ")"
	case token.XOR:
		if isTypeParam(t) {
			return fmt.Sprintf("$rt.notT(%s, %s)", fe.desc(t), fe.expr(e.X))
		}
		ii, _ := intKind(t)
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
	if tv, ok := fe.info.Types[fun]; ok && tv.IsType() {
		return fe.conversion(e, tv.Type)
	}
	if id, ok := fun.(*ast.Ident); ok {
		if b, ok := fe.info.Uses[id].(*types.Builtin); ok {
			return fe.builtin(e, b.Name())
		}
	}
	if se, ok := fun.(*ast.SelectorExpr); ok {
		if b, ok := fe.info.Uses[se.Sel].(*types.Builtin); ok { // unsafe.X
			return fe.unsafeCall(e, b.Name())
		}
	}
	sig := under(fe.info.TypeOf(e.Fun)).(*types.Signature)
	args := fe.args(e, sig)
	var callee string
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		if sel, ok := fe.info.Selections[f]; ok && sel.Kind() == types.MethodVal {
			prefix, recv, iface := fe.methodTarget(f, sel)
			fn := sel.Obj().(*types.Func)
			if iface {
				callee = fmt.Sprintf("$rt.icall(%s, %s", recv, jsString(methodKey(fn)))
				if args != "" {
					callee += ", " + args
				}
				callee += ")"
			} else {
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
			}
		}
	}
	if callee == "" {
		callee = fe.expr(fun)
		if _, ok := fun.(*ast.FuncLit); ok {
			callee = "(" + callee + ")"
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
	if fe.pe.prog.CallBlocks(fe.info, call) {
		return "(await " + s + ")"
	}
	return s
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
	var vals []string
	if len(e.Args) == 1 && n > 1 {
		if tt, ok := fe.info.TypeOf(e.Args[0]).(*types.Tuple); ok {
			t := fe.tmp()
			var parts []string
			for i := 0; i < tt.Len(); i++ {
				parts = append(parts, fe.convert(fmt.Sprintf("%s[%d]", t, i), tt.At(i).Type(), paramType(i)))
			}
			if sig.Variadic() {
				fixed := parts[:n-1]
				rest := parts[n-1:]
				parts = append(fixed, "$rt.sliceLit(["+strings.Join(rest, ", ")+"])")
			}
			return fmt.Sprintf("...((%s: any) => [%s])(%s)", t, strings.Join(parts, ", "), fe.expr(e.Args[0]))
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
			vals = append(fixed, "$rt.sliceLit(["+strings.Join(rest, ", ")+"])")
		}
	}
	return strings.Join(vals, ", ")
}

func (fe *funcEmitter) conversion(e *ast.CallExpr, to types.Type) string {
	arg := e.Args[0]
	from := fe.info.TypeOf(arg)
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
			from = types.Default(from)
		}
		return fmt.Sprintf("$rt.convertT(%s, %s, %s)", fe.desc(to), fe.desc(from), s)
	}
	if tb, ok := tu.(*types.Basic); ok {
		fb, _ := fu.(*types.Basic)
		switch {
		case tb.Info()&types.IsString != 0:
			if fb != nil && fb.Info()&types.IsInteger != 0 {
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
			if fb != nil && fb.Info()&types.IsFloat != 0 {
				return wrap("$rt.trunc("+s+")", ii)
			}
			if fi, ok := intKind(from); ok && (fi.bits > ii.bits || fi.signed != ii.signed || ii.bits < 64) {
				return wrap(s, ii)
			}
			return s
		case tb.Kind() == types.Float32:
			return "$rt.fround(" + s + ")"
		case tb.Info()&types.IsFloat != 0:
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
				fe.errorf(e.Pos(), "conversion from slice to array pointer is not supported yet")
				return s
			}
		}
	}
	if st, ok := tu.(*types.Struct); ok && !types.Identical(to, from) && !isGenericType(to) && !isGenericType(from) {
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
	arg := func(i int) string { return fe.expr(e.Args[i]) }
	m := fe.mark(e)
	switch name {
	case "len", "cap":
		t := fe.info.TypeOf(e.Args[0])
		if p, ok := under(t).(*types.Pointer); ok {
			t = p.Elem()
		}
		switch u := under(t).(type) {
		case *types.Basic:
			return arg(0) + ".length"
		case *types.Slice:
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
		if e.Ellipsis.IsValid() {
			src := arg(1)
			if b, ok := under(fe.info.TypeOf(e.Args[1])).(*types.Basic); ok && b.Info()&types.IsString != 0 {
				src = "$rt.stringToBytes(" + src + ")"
			}
			return fmt.Sprintf("%s$rt.append(%s, $rt.toArray(%s), %s%s)", m, arg(0), src, fe.zeroFn(elem), fe.elemTypeArg(elem))
		}
		var vals []string
		for _, a := range e.Args[1:] {
			vals = append(vals, fe.valueOf(a, elem))
		}
		return fmt.Sprintf("%s$rt.append(%s, [%s], %s%s)", m, arg(0), strings.Join(vals, ", "), fe.zeroFn(elem), fe.elemTypeArg(elem))
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
			l, c := arg(1), "undefined"
			if len(e.Args) > 2 {
				c = arg(2)
			}
			return fmt.Sprintf("%s$rt.makeSlice(%s, %s, %s)", m, l, c, fe.zeroFn(u.Elem()))
		case *types.Map:
			if len(e.Args) > 1 { // the size hint is evaluated, then unused
				return fmt.Sprintf("%s(%s, $rt.makeMap(%s))", m, arg(1), fe.desc(u.Key()))
			}
			return fmt.Sprintf("%s$rt.makeMap(%s)", m, fe.desc(u.Key()))
		case *types.Chan:
			c := "0"
			if len(e.Args) > 1 {
				c = arg(1)
			}
			return fmt.Sprintf("%s$rt.makeChan(%s, %s)", m, c, fe.zeroFn(u.Elem()))
		}
	case "new":
		t := fe.info.TypeOf(e.Args[0])
		if _, isTP := types.Unalias(t).(*types.TypeParam); isTP {
			return "$rt.newPtr(" + fe.desc(t) + ")"
		}
		if isAggregate(t) {
			return fe.zero(t)
		}
		return "$rt.cell(" + fe.zero(t) + ")"
	case "panic":
		return fmt.Sprintf("%s$rt.panic(%s)", m, fe.valueOf(e.Args[0], types.Universe.Lookup("any").Type()))
	case "recover":
		return m + "$rt.recover()"
	case "print", "println":
		var vals []string
		for i := range e.Args {
			vals = append(vals, arg(i))
		}
		return m + "$rt.println(" + strings.Join(vals, ", ") + ")"
	case "close":
		return m + "$rt.close(" + arg(0) + ")"
	case "clear":
		t := fe.info.TypeOf(e.Args[0])
		if sl, ok := under(t).(*types.Slice); ok {
			return fmt.Sprintf("%s$rt.sliceClear(%s, %s%s)", m, arg(0), fe.zeroFn(sl.Elem()), fe.elemTypeArg(sl.Elem()))
		}
		return m + "$rt.mapClear(" + arg(0) + ")"
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
// the identity. There is no address space, so reinterpreting memory as
// another type, pointer arithmetic and conversions to and from uintptr are
// diagnosed. unsafe.String and unsafe.Slice are supported where their pointer
// operand is an element of a slice or array (&x[i], unsafe.SliceData(x)).

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
		fe.errorf(e.Pos(), "conversion from %s to unsafe.Pointer is not supported (goesm has no address space)", from)
		return s
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
				fe.errorf(e.Pos(), "reinterpreting %s as %s through unsafe.Pointer is not supported", up, to)
			}
		}
	}
	return s
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
				return fe.expr(ix.X), fe.expr(ix.Index), true, true
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
			return fmt.Sprintf("%s$rt.bytesToString($rt.unsafeSlice(%s, %s, %s, %v))", m, base, idx, fe.expr(e.Args[1]), checked)
		}
	case "Slice":
		if base, idx, checked, ok := fe.unsafeElem(e.Args[0]); ok {
			return fmt.Sprintf("%s$rt.unsafeSlice(%s, %s, %s, %v)", m, base, idx, fe.expr(e.Args[1]), checked)
		}
		if c, ok := unparen(e.Args[0]).(*ast.CallExpr); ok {
			if se, ok := unparen(c.Fun).(*ast.SelectorExpr); ok {
				if b, ok := fe.info.Uses[se.Sel].(*types.Builtin); ok && b.Name() == "StringData" {
					// The bytes of a string are immutable, so a copy is
					// indistinguishable from an alias.
					return fmt.Sprintf("%s$rt.stringToBytes($rt.substr(%s, 0, %s))", m, fe.expr(c.Args[0]), fe.expr(e.Args[1]))
				}
			}
		}
	case "Sizeof", "Alignof": // not constant: the operand's type involves a type parameter
		return fmt.Sprintf("%s$rt.%sOf(%s)", m, strings.ToLower(name[:len(name)-2]), fe.desc(fe.info.TypeOf(e.Args[0])))
	case "SliceData":
		return fmt.Sprintf("%s$rt.sliceData(%s, %v)", m, fe.expr(e.Args[0]), isAggregate(under(fe.info.TypeOf(e.Args[0])).(*types.Slice).Elem()))
	}
	fe.errorf(e.Pos(), "unsafe.%s is not supported in this form (goesm has no address space)", name)
	return "undefined"
}
