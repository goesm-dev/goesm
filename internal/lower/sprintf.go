package lower

import (
	"go/ast"
	"go/constant"
	"go/types"
	"math/big"
	"strconv"
	"strings"
)

// fmt.Sprintf with a constant format whose verbs all apply to arguments of
// predeclared types is lowered to string concatenation, as one would write
// it in JS:
//
//	fmt.Sprintf("%d:%s|%.2f", i, s, f)
//
// becomes
//
//	(f1 = $rt.fmtF(f, 2)) === "" || !Number.isSafeInteger(i)
//		? fmt.Sprintf("%d:%s|%.2f", $rt.sliceLit([...]))
//		: "" + i + ":" + s + "|" + f1
//
// Boxing the arguments into a []any and walking the format at run time
// cost more than formatting them. The conditions select the values whose
// JS formatting is not Go's (an int past 2^53, a float that toFixed does
// not format as strconv does), which take fmt.Sprintf itself.
//
// The verbs %v, %s and, for fmt.Errorf, one %w of an operand of type error
// are its Error method's result, which $rt.errText returns after checking
// that fmt would print just that (the error is not nil and has no Format
// method). Those checks come after every other condition, and with more
// than one error operand they are made for all of them before any Error is
// called, so that no method is called twice. %q of a string quotes it when
// it is printable ASCII. fmt.Errorf becomes a call of the fmt patch's
// newError with the message and the %w operand.

// sprintfSpec is a verb of a format, with the literal text before it.
type sprintfSpec struct {
	lit  string
	prec int // -1 if none
	verb byte
}

// parseSprintf parses a format into its verbs and the text after the last
// one. ok is false for a format with anything but verbs without flags or a
// width, a precision only for %f and %F.
func parseSprintf(format string) (specs []sprintfSpec, tail string, ok bool) {
	var lit strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			lit.WriteByte(c)
			continue
		}
		i++
		if i >= len(format) {
			return nil, "", false
		}
		if format[i] == '%' {
			lit.WriteByte('%')
			continue
		}
		prec := -1
		if format[i] == '.' {
			prec = 0
			for i++; i < len(format) && '0' <= format[i] && format[i] <= '9'; i++ {
				prec = prec*10 + int(format[i]-'0')
				if prec > 100 {
					return nil, "", false
				}
			}
			if i >= len(format) {
				return nil, "", false
			}
		}
		verb := format[i]
		if !strings.ContainsRune("vdxXstfFqw", rune(verb)) || (prec >= 0 && verb != 'f' && verb != 'F') {
			return nil, "", false
		}
		specs = append(specs, sprintfSpec{lit: lit.String(), prec: prec, verb: verb})
		lit.Reset()
	}
	return specs, lit.String(), true
}

// sprintfArgType returns the type an argument of Sprintf is boxed with, if
// it is a predeclared type that the lowering formats.
func sprintfArgType(t types.Type) (*types.Basic, bool) {
	b, ok := types.Unalias(t).(*types.Basic)
	if !ok {
		return nil, false
	}
	if b.Info()&types.IsUntyped != 0 {
		if b.Kind() == types.UntypedNil {
			return nil, false
		}
		b = types.Default(b).(*types.Basic)
	}
	switch b.Kind() {
	case types.String, types.Bool, types.Float64,
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		return b, true
	}
	return nil, false
}

// sprintfVerbOK reports whether the lowering formats an argument of type b
// with verb as Go does.
func sprintfVerbOK(b *types.Basic, verb byte) bool {
	switch {
	case b.Kind() == types.String:
		return verb == 's' || verb == 'v' || verb == 'q'
	case b.Kind() == types.Bool:
		return verb == 't' || verb == 'v'
	case b.Kind() == types.Float64:
		return verb == 'f' || verb == 'F' || verb == 'v'
	default: // integers
		return verb == 'd' || verb == 'v' || verb == 'x' || verb == 'X'
	}
}

// fmtFunc returns the function of package fmt that e calls, if it is
// Sprintf or Errorf.
func (fe *funcEmitter) fmtFunc(e *ast.CallExpr) *types.Func {
	sel, ok := ast.Unparen(e.Fun).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	fn, ok := fe.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Signature().Recv() != nil || (fn.Name() != "Sprintf" && fn.Name() != "Errorf") {
		return nil
	}
	return fn
}

// isFmtHelper reports whether fn is a function of the fmt patch that the
// lowering calls from other packages, which fmt exports for them.
func isFmtHelper(fn *types.Func) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == "fmt" && (fn.Name() == "newError" || fn.Name() == "panicText")
}

// fmtHelper returns the JS reference to the fmt patch's function name, or
// "" if fmt has none.
func (fe *funcEmitter) fmtHelper(fn *types.Func, name string) string {
	f, ok := fn.Pkg().Scope().Lookup(name).(*types.Func)
	if !ok || !isFmtHelper(f) {
		return ""
	}
	return fe.nameOf(f)
}

// sprintf lowers a call of fmt.Sprintf or fmt.Errorf as described above,
// or returns false.
func (fe *funcEmitter) sprintf(e *ast.CallExpr) (string, bool) {
	if !fe.inBody || e.Ellipsis.IsValid() || len(e.Args) == 0 {
		return "", false
	}
	fn := fe.fmtFunc(e)
	if fn == nil || fe.callBlocks(e) {
		return "", false
	}
	errorf := fn.Name() == "Errorf"
	ftv := fe.info.Types[e.Args[0]]
	if ftv.Value == nil || ftv.Value.Kind() != constant.String {
		return "", false
	}
	specs, tail, ok := parseSprintf(constant.StringVal(ftv.Value))
	args := e.Args[1:]
	if !ok || len(specs) != len(args) {
		return "", false
	}
	errT := types.Universe.Lookup("error").Type()
	bts := make([]*types.Basic, len(args)) // nil for an error
	nerr, wrapped := 0, -1
	for i, a := range args {
		verb := specs[i].verb
		if t := fe.info.TypeOf(a); types.Identical(t, errT) {
			switch {
			case verb == 'v' || verb == 's':
			case verb == 'w' && errorf && wrapped < 0:
				wrapped = i
			default:
				return "", false
			}
			nerr++
			continue
		}
		b, ok := sprintfArgType(fe.info.TypeOf(a))
		if !ok || !sprintfVerbOK(b, verb) {
			return "", false
		}
		bts[i] = b
	}
	var newError, panicText string
	if errorf {
		if newError = fe.fmtHelper(fn, "newError"); newError == "" {
			return "", false
		}
	}
	if nerr > 0 {
		if panicText = fe.fmtHelper(fn, "panicText"); panicText == "" {
			return "", false
		}
	}

	// The pieces of the result: literal text (folded with constant
	// arguments) and JS expressions.
	var pieces []string
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			pieces = append(pieces, jsString(lit.String()))
			lit.Reset()
		}
	}
	temp := func() string {
		t := fe.declareName("$f")
		fe.temps = append(fe.temps, t)
		return t
	}
	var sets, conds, errChecks, errConds, boxes []string
	wrappedJS := "null"
	anyT := types.Universe.Lookup("any").Type()
	for i, a := range args {
		sp, b := specs[i], bts[i]
		lit.WriteString(sp.lit)
		boxes = append(boxes, fe.valueOf(a, anyT))
		if b != nil {
			if s, ok := sprintfConst(fe.info.Types[a].Value, b, sp.verb); ok {
				lit.WriteString(s)
				continue
			}
		}
		v := stripMarks(fe.expr(a))
		// An Error method may change a variable, so with an error among
		// the operands all are evaluated first, as fmt sees them.
		if !reusable(v) || nerr > 0 && fe.info.Types[a].Value == nil {
			t := temp()
			sets = append(sets, t+" = "+v)
			v = t
		}
		var piece string
		switch {
		case b == nil: // an error
			boxes[i] = fe.convert(v, errT, anyT)
			if i == wrapped {
				wrappedJS = v
			}
			if nerr > 1 {
				errChecks = append(errChecks, "!$rt.plainErr("+v+")")
			}
			piece = temp()
			errConds = append(errConds, "("+piece+" = $rt.errText("+v+", "+strconv.Itoa(int(sp.verb))+", "+panicText+")) === null")
		case b.Kind() == types.Float64 && sp.verb == 'v':
			piece = "$rt.fmtShortest(" + v + ")"
		case b.Kind() == types.Float64:
			prec := sp.prec
			if prec < 0 {
				prec = 6
			}
			piece = temp()
			conds = append(conds, "("+piece+" = $rt.fmtF("+v+", "+strconv.Itoa(prec)+")) === \"\"")
		case b.Kind() == types.String && sp.verb == 'q':
			piece = temp()
			conds = append(conds, "("+piece+" = $rt.quoteText("+v+")) === null")
		case b.Kind() == types.String || b.Kind() == types.Bool:
			piece = v
		default: // integers
			if b.Kind() == types.Int || b.Kind() == types.Uint || b.Kind() == types.Uintptr {
				conds = append(conds, "!Number.isSafeInteger("+v+")")
			}
			switch sp.verb {
			case 'x':
				piece = v + ".toString(16)"
			case 'X':
				piece = v + ".toString(16).toUpperCase()"
			default:
				piece = v
			}
		}
		if b != nil {
			boxes[i] = fe.convert(v, b, anyT)
		}
		flush()
		pieces = append(pieces, piece)
	}
	lit.WriteString(tail)
	flush()
	if len(pieces) == 0 {
		pieces = []string{`""`}
	} else if !strings.HasPrefix(pieces[0], `"`) {
		pieces = append([]string{`""`}, pieces...)
	}
	s := strings.Join(pieces, " + ")
	if errorf {
		s = newError + "(" + s + ", " + wrappedJS + ")"
	}
	conds = append(append(conds, errChecks...), errConds...)
	if len(conds) > 0 {
		call := fe.mark(e) + fe.expr(e.Fun) + "(" + fe.expr(e.Args[0]) + ", $rt.sliceLit<$rt.Iface | null>([" + strings.Join(boxes, ", ") + "]))"
		s = strings.Join(conds, " || ") + " ? " + call + " : " + s
	}
	if len(sets) > 0 {
		s = strings.Join(sets, ", ") + ", " + s
	}
	return "(" + s + ")", true
}

// sprintfConst formats the constant v of type b for verb, if the lowering
// folds it into the format's text.
func sprintfConst(v constant.Value, b *types.Basic, verb byte) (string, bool) {
	if v == nil {
		return "", false
	}
	switch {
	case b.Kind() == types.String && verb == 'q':
		return strconv.Quote(constant.StringVal(v)), true
	case b.Kind() == types.String:
		return constant.StringVal(v), true
	case b.Kind() == types.Bool:
		if constant.BoolVal(v) {
			return "true", true
		}
		return "false", true
	case b.Info()&types.IsInteger != 0:
		v = constant.ToInt(v)
		if v.Kind() != constant.Int {
			return "", false
		}
		switch verb {
		case 'd', 'v':
			return v.ExactString(), true
		case 'x', 'X':
			var s string
			switch x := constant.Val(v).(type) {
			case int64:
				s = strconv.FormatInt(x, 16) // -ff for -255, as Go's
			case *big.Int:
				s = x.Text(16)
			}
			if verb == 'X' {
				s = strings.ToUpper(s)
			}
			return s, true
		}
	}
	return "", false
}
