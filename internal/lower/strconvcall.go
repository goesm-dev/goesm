package lower

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// A call of strconv.FormatFloat or AppendFloat that names bitSize 64 and a
// format 'e', 'E', 'f', 'g' or 'G', and a call of strconv.ParseFloat that
// names bitSize 64, convert with the engine's number formatting and
// parsing (formatFloat64 and parseFloat64 in runtime/src/fmt.ts, through
// the strconv patch's number.go). The call no longer refers to
// internal/strconv's conversions, so a program that converts only such
// floats leaves out their code and tables, about 25 KB minified.

// strconvCall lowers such a call, or returns false.
func (fe *funcEmitter) strconvCall(e *ast.CallExpr) (string, bool) {
	if e.Ellipsis.IsValid() {
		return "", false
	}
	sel, ok := unparen(e.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	fn, ok := fe.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "strconv" || fn.Signature().Recv() != nil {
		return "", false
	}
	constInt := func(a ast.Expr) (int64, bool) {
		tv := fe.info.Types[a]
		if tv.Value == nil || tv.Value.Kind() != constant.Int {
			return 0, false
		}
		return constant.Int64Val(tv.Value)
	}
	float64Format := func(a ast.Expr) bool {
		c, ok := constInt(a)
		return ok && c < 128 && strings.ContainsRune("eEfgG", rune(c))
	}
	native := func(name string) string {
		fe.pe.usesNatives = true
		return "$natives." + goesmruntime.NativeName("strconv."+name)
	}
	f64, i := types.Typ[types.Float64], types.Typ[types.Int]
	switch fn.Name() {
	case "FormatFloat":
		if len(e.Args) != 4 || !float64Format(e.Args[1]) {
			return "", false
		}
		if bits, ok := constInt(e.Args[3]); !ok || bits != 64 {
			return "", false
		}
		return fe.mark(e) + native("formatFloat64JS") + "(" + fe.valueOf(e.Args[0], f64) + ", " + fe.expr(e.Args[1]) + ", " + fe.valueOf(e.Args[2], i) + ")", true
	case "AppendFloat":
		if len(e.Args) != 5 || !float64Format(e.Args[2]) {
			return "", false
		}
		if bits, ok := constInt(e.Args[4]); !ok || bits != 64 {
			return "", false
		}
		dst := fe.valueOf(e.Args[0], fe.info.TypeOf(e.Args[0]))
		return fe.mark(e) + "$rt.appendString(" + dst + ", " + native("formatFloat64JS") + "(" + fe.valueOf(e.Args[1], f64) + ", " + fe.expr(e.Args[2]) + ", " + fe.valueOf(e.Args[3], i) + "))", true
	case "ParseFloat":
		if len(e.Args) != 2 {
			return "", false
		}
		if bits, ok := constInt(e.Args[1]); !ok || bits != 64 {
			return "", false
		}
		h, ok := fn.Pkg().Scope().Lookup("parseFloat64").(*types.Func)
		if !ok || !isLoweringHelper(h) {
			return "", false
		}
		return fe.mark(e) + fe.nameOf(h) + "(" + fe.valueOf(e.Args[0], types.Typ[types.String]) + ")", true
	}
	return "", false
}
