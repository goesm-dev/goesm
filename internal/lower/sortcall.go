package lower

import (
	"go/ast"
	"go/types"

	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// A call of slices.Sort (also through sort.Ints and sort.Strings) whose
// element type is an integer or string type where it is called sorts with
// the engine's built-in sort (sortBuiltin in the slices patch) directly. The
// call no longer refers to Sort, so a program that sorts only integers and
// strings leaves out pdqsortOrdered, about 7 KB minified, which Sort keeps
// for floating-point elements.

// sortCall lowers such a call, or returns false.
func (fe *funcEmitter) sortCall(e *ast.CallExpr) (string, bool) {
	if len(e.Args) != 1 || e.Ellipsis.IsValid() {
		return "", false
	}
	var id *ast.Ident
	switch f := unparen(funcIdent(unparen(e.Fun))).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return "", false
	}
	if fn, ok := fe.info.Uses[id].(*types.Func); !ok || fn.FullName() != "slices.Sort" {
		return "", false
	}
	inst, ok := fe.info.Instances[id]
	if !ok || inst.TypeArgs.Len() != 2 || hasTypeParam(inst.TypeArgs.At(1)) {
		return "", false
	}
	b, ok := inst.TypeArgs.At(1).Underlying().(*types.Basic)
	if !ok || b.Info()&(types.IsInteger|types.IsString) == 0 {
		return "", false
	}
	fe.pe.usesNatives = true
	at := fe.info.TypeOf(e.Args[0])
	box := fe.convert(fe.expr(e.Args[0]), at, types.Universe.Lookup("any").Type())
	return fe.mark(e) + "$natives." + goesmruntime.NativeName("slices.sortBuiltin") + "(" + box + ")", true
}
