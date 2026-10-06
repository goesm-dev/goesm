package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// encoding/json calls between JS strings. The runtime's encoder and decoder
// work on JS strings; json.Unmarshal([]byte(s), v) would convert s to bytes
// for Unmarshal to convert back, and the result of json.Marshal often only
// becomes a string:
//
//	b, err := json.Marshal(v)
//	...
//	return string(b)
//
// json.Unmarshal of a conversion []byte(s) passes s to the runtime as it is,
// and a pointer of a known type without boxing it.
// A []byte local defined by json.Marshal whose every use is string(b) or
// len(b) is kept as the Go string the runtime encodes, which those uses read
// directly. Values the runtime leaves to Go's code (types with methods, for
// one) go through json.Marshal and json.Unmarshal themselves.

// isJSONFunc reports whether e calls encoding/json's function name.
func isJSONFunc(info *types.Info, e *ast.CallExpr, name string) bool {
	sel, ok := ast.Unparen(e.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "encoding/json" && fn.Name() == name && fn.Signature().Recv() == nil
}

// stringBytesVars returns the []byte locals of body defined by json.Marshal
// and used only as string(b) and len(b), with the calls that define them.
func stringBytesVars(info *types.Info, body *ast.BlockStmt, boxed map[*types.Var]bool) (map[*types.Var]bool, map[*ast.CallExpr]bool) {
	if body == nil || containsGoto(body) {
		return nil, nil
	}
	defs := map[*types.Var]*ast.CallExpr{}
	ok := map[*ast.Ident]bool{} // uses as string(b) or len(b)
	var uses []*ast.Ident
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE || len(n.Lhs) != 2 || len(n.Rhs) != 1 {
				break
			}
			call, isCall := ast.Unparen(n.Rhs[0]).(*ast.CallExpr)
			id, isID := n.Lhs[0].(*ast.Ident)
			if !isCall || !isID || call.Ellipsis.IsValid() || !isJSONFunc(info, call, "Marshal") {
				break
			}
			if v, isVar := info.Defs[id].(*types.Var); isVar && isByteSlice(v.Type()) && !boxed[v] {
				defs[v] = call
			}
		case *ast.CallExpr:
			if len(n.Args) != 1 {
				break
			}
			id, isID := ast.Unparen(n.Args[0]).(*ast.Ident)
			if !isID {
				break
			}
			if tv, isType := info.Types[ast.Unparen(n.Fun)]; isType && tv.IsType() {
				if b, isBasic := tv.Type.Underlying().(*types.Basic); isBasic && b.Info()&types.IsString != 0 {
					ok[id] = true
				}
			} else if fid, isFID := ast.Unparen(n.Fun).(*ast.Ident); isFID {
				if b, isB := info.Uses[fid].(*types.Builtin); isB && b.Name() == "len" {
					ok[id] = true
				}
			}
		case *ast.Ident:
			uses = append(uses, n)
		}
		return true
	})
	if len(defs) == 0 {
		return nil, nil
	}
	for _, id := range uses {
		if v, isVar := info.Uses[id].(*types.Var); isVar && !ok[id] {
			delete(defs, v)
		}
	}
	if len(defs) == 0 {
		return nil, nil
	}
	vars, calls := map[*types.Var]bool{}, map[*ast.CallExpr]bool{}
	for v, call := range defs {
		vars[v], calls[call] = true, true
	}
	return vars, calls
}

// stringBytesRead returns the string held by a []byte local that
// stringBytesVars kept as one, for string(b) and len(b).
func (fe *funcEmitter) stringBytesRead(arg ast.Expr) (string, bool) {
	id, ok := ast.Unparen(arg).(*ast.Ident)
	if !ok || fe.pe.strBytes == nil {
		return "", false
	}
	v, ok := fe.info.Uses[id].(*types.Var)
	if !ok || !fe.pe.strBytes[v] {
		return "", false
	}
	return fe.declare(v), true
}

// jsonCall lowers the calls of encoding/json described above, or returns
// false.
func (fe *funcEmitter) jsonCall(e *ast.CallExpr) (string, bool) {
	if !fe.inBody || e.Ellipsis.IsValid() || len(e.Args) == 0 {
		return "", false
	}
	anyT := types.Universe.Lookup("any").Type()
	temp := func(s string) (set, ref string) {
		if s = stripMarks(s); reusable(s) {
			return "", s
		}
		t := fe.declareName("$j")
		fe.temps = append(fe.temps, t)
		return t + " = " + s + ", ", t
	}
	if fe.pe.strMarshal[e] || len(e.Args) == 1 && isJSONFunc(fe.info, e, "Marshal") {
		// A value of a type with a generated encoder (see jsonenc.go) is
		// encoded by it, and boxed for the runtime's encoder and Go's code
		// only when that gives up.
		t := fe.info.TypeOf(e.Args[0])
		typed := jsonEncComposite(t)
		if !typed && !fe.pe.strMarshal[e] || fe.callBlocks(e) {
			return "", false
		}
		s := fe.declareName("$j")
		fe.temps = append(fe.temps, s)
		if typed {
			setR, r := temp(fe.expr(e.Args[0]))
			f := fe.pe.jsonEncoder(t)
			box := fe.convertCopy(r, t, anyT)
			call := fe.mark(e) + fe.expr(e.Fun) + "(" + box + ")"
			if !fe.pe.strMarshal[e] {
				return "(" + setR + "(" + s + " = $rt.jsonMarshalWith(" + f + ", " + r + ")) !== null ? [" + s + ", null] : " + call + ")", true
			}
			return "(" + setR + "(" + s + " = $rt.jsonMarshalStringWith(" + f + ", " + r + ")) !== null || (" +
				s + " = $rt.jsonMarshalString(" + box + ")) !== null ? [" + s + ", null] : (" +
				s + " = " + call + ", [$rt.bytesToString(" + s + "[0]), " + s + "[1]]))", true
		}
		set, x := temp(fe.valueOf(e.Args[0], anyT))
		call := fe.mark(e) + fe.expr(e.Fun) + "(" + x + ")"
		return "(" + set + "(" + s + " = $rt.jsonMarshalString(" + x + ")) !== null ? [" + s + ", null] : (" +
			s + " = " + call + ", [$rt.bytesToString(" + s + "[0]), " + s + "[1]]))", true
	}
	if len(e.Args) != 2 || !isJSONFunc(fe.info, e, "Unmarshal") || fe.callBlocks(e) {
		return "", false
	}
	conv, ok := ast.Unparen(e.Args[0]).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 {
		return "", false
	}
	if tv, ok := fe.info.Types[ast.Unparen(conv.Fun)]; !ok || !tv.IsType() || !isByteSlice(tv.Type.Underlying()) {
		return "", false
	}
	if b, ok := fe.info.TypeOf(conv.Args[0]).Underlying().(*types.Basic); !ok || b.Info()&types.IsString == 0 {
		return "", false
	}
	setS, s := temp(fe.expr(conv.Args[0]))
	if pt, ok := fe.info.TypeOf(e.Args[1]).(*types.Pointer); ok {
		// A pointer goes to the runtime as it is, and is boxed only for Go's
		// code.
		setP, p := temp(fe.expr(e.Args[1]))
		call := fe.mark(e) + fe.expr(e.Fun) + "($rt.stringToBytes(" + s + "), " + fe.convert(p, pt, anyT) + ")"
		return "(" + setS + setP + "$rt.jsonUnmarshalTo(" + s + ", " + fe.desc(pt) + ", " + p + ") ? null : " + call + ")", true
	}
	setV, v := temp(fe.valueOf(e.Args[1], anyT))
	call := fe.mark(e) + fe.expr(e.Fun) + "($rt.stringToBytes(" + s + "), " + v + ")"
	return "(" + setS + setV + "$rt.jsonUnmarshalString(" + s + ", " + v + ") ? null : " + call + ")", true
}
