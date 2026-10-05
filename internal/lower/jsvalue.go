package lower

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"
	"unicode/utf8"
)

// syscall/js calls. Value.Call, Invoke and New take their arguments as
// ...any, and Set and SetIndex take an any, which js.ValueOf converts with a
// type switch: a call boxes every argument, allocates the slice and switches
// on each type. Where every argument has a type ValueOf converts directly (a
// predeclared boolean, number or string type, js.Value, js.Func or nil), the
// call is lowered to the package's getJS, callJS, invokeJS, newJS, setJS or
// setIndexJS with the arguments converted inline from their static types (a
// JavaScript array of them for the calls; internal/natives/goroot/syscall/js).
// Other calls, and calls with a spread argument (f(args...)), take the
// general path.
// Get, Set and Call also take the property name as a JavaScript string, so
// a constant ASCII name, the usual case, is passed without decoding it from
// UTF-8 on each call.

var jsValueCalls = map[string]struct {
	method string
	fixed  int  // leading arguments that are not converted (Call's method name)
	array  bool // the converted arguments are passed as an array
	name   bool // the first argument is a property name, passed as a JavaScript string
}{
	"(syscall/js.Value).Get":      {"getJS", 1, false, true},
	"(syscall/js.Value).Call":     {"callJS", 1, true, true},
	"(syscall/js.Value).Invoke":   {"invokeJS", 0, true, false},
	"(syscall/js.Value).New":      {"newJS", 0, true, false},
	"(syscall/js.Value).Set":      {"setJS", 1, false, true},
	"(syscall/js.Value).SetIndex": {"setIndexJS", 1, false, false},
}

func (fe *funcEmitter) jsValueCall(e *ast.CallExpr) (string, bool) {
	sel, ok := unparen(e.Fun).(*ast.SelectorExpr)
	if !ok || e.Ellipsis.IsValid() {
		return "", false
	}
	if len(e.Args) == 1 {
		if _, ok := fe.info.TypeOf(e.Args[0]).(*types.Tuple); ok {
			return "", false // f(g()) with several results of g
		}
	}
	s := fe.info.Selections[sel]
	if s == nil || s.Kind() != types.MethodVal {
		return "", false
	}
	fn := s.Obj().(*types.Func)
	c, ok := jsValueCalls[fn.FullName()]
	if !ok || !types.Identical(fe.info.TypeOf(sel.X), fn.Signature().Recv().Type()) {
		return "", false
	}
	value := fn.Signature().Recv().Type()
	jsFunc := fn.Pkg().Scope().Lookup("Func").Type()
	var conv []string
	for _, a := range e.Args[c.fixed:] {
		s, ok := fe.jsArg(a, value, jsFunc)
		if !ok {
			return "", false
		}
		conv = append(conv, s)
	}
	parts := []string{fe.expr(sel.X)}
	for i, a := range e.Args[:c.fixed] {
		if i == 0 && c.name {
			parts = append(parts, fe.jsString(a))
			continue
		}
		parts = append(parts, fe.valueOf(a, fn.Signature().Params().At(i).Type()))
	}
	if c.array {
		parts = append(parts, "["+strings.Join(conv, ", ")+"]")
	} else {
		parts = append(parts, conv...)
	}
	return fe.awaitIf(e, fe.mark(e)+fe.pe.qualify(fn.Pkg(), "Value$"+c.method)+"("+strings.Join(parts, ", ")+")"), true
}

// jsArg is argument a converted as js.ValueOf converts it, or false if its
// type is not one ValueOf converts without its type switch. value and
// jsFunc are js.Value and js.Func.
func (fe *funcEmitter) jsArg(a ast.Expr, value, jsFunc types.Type) (string, bool) {
	t := types.Unalias(fe.info.TypeOf(a))
	if types.Identical(t, value) {
		return "$rt.fromRef((" + fe.expr(a) + ").ref)", true
	}
	if types.Identical(t, jsFunc) {
		return "$rt.fromRef((" + fe.expr(a) + ").Value.ref)", true
	}
	b, ok := t.(*types.Basic)
	if !ok {
		return "", false
	}
	if b.Kind() == types.UntypedNil {
		return "null", true
	}
	if b.Info()&types.IsUntyped != 0 {
		b = types.Default(b).(*types.Basic)
	}
	switch b.Kind() {
	case types.Bool, types.Int, types.Int8, types.Int16, types.Int32, types.Uint, types.Uint8, types.Uint16, types.Uint32,
		types.Uintptr, types.Float32, types.Float64:
		return fe.valueOf(a, b), true
	case types.Int64, types.Uint64:
		return "Number(" + fe.valueOf(a, b) + ")", true
	case types.String:
		return fe.jsString(a), true
	}
	return "", false
}

// jsString is the string a converted to a JavaScript string: the constant
// itself if it is ASCII, which the two representations share.
func (fe *funcEmitter) jsString(a ast.Expr) string {
	if tv := fe.info.Types[a]; tv.Value != nil && tv.Value.Kind() == constant.String && isASCII(constant.StringVal(tv.Value)) {
		return fe.expr(a)
	}
	return "$rt.toJSString(" + fe.valueOf(a, types.Typ[types.String]) + ")"
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
