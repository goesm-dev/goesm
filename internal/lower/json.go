package lower

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strings"
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
// and a pointer of a known type without boxing it. When the runtime decodes
// every value of that type itself, errors included (jsonDecodes), the call
// does not refer to encoding/json at all, and a module whose calls all do
// so leaves out its import (elided in emit.go).
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
				return "(" + setR + "(" + s + " = $rt.jsonMarshalWith(" + f + ", " + r + ")) !== null ? ($rt.$R.r1 = null, " + s + ") : " + call + ")", true
			}
			return "(" + setR + "(" + s + " = $rt.jsonMarshalStringWith(" + f + ", " + r + ")) !== null || (" +
				s + " = $rt.jsonMarshalString(" + box + ")) !== null ? ($rt.$R.r1 = null, " + s + ") : (" +
				"$rt.bytesToString(" + call + ")))", true
		}
		set, x := temp(fe.valueOf(e.Args[0], anyT))
		call := fe.mark(e) + fe.expr(e.Fun) + "(" + x + ")"
		return "(" + set + "(" + s + " = $rt.jsonMarshalString(" + x + ")) !== null ? ($rt.$R.r1 = null, " + s + ") : (" +
			"$rt.bytesToString(" + call + ")))", true
	}
	if len(e.Args) != 2 || !isJSONFunc(fe.info, e, "Unmarshal") || fe.callBlocks(e) {
		return "", false
	}
	// The runtime decodes JSON text in a JS string: s of []byte(s) as it
	// is, other []byte values converted.
	var setS, s, data string
	conv, isConv := ast.Unparen(e.Args[0]).(*ast.CallExpr)
	if isConv = isConv && len(conv.Args) == 1 && fe.isStringToBytes(conv); isConv {
		setS, s = temp(fe.expr(conv.Args[0]))
		data = "$rt.stringToBytes(" + s + ")"
	} else {
		var b string
		setS, b = temp(fe.expr(e.Args[0]))
		s, data = "$rt.bytesToString("+b+")", b
	}
	if pt, ok := fe.info.TypeOf(e.Args[1]).(*types.Pointer); ok {
		// A pointer goes to the runtime as it is, and is boxed only for Go's
		// code, which a value the runtime always decodes itself does not
		// need.
		setP, p := temp(fe.expr(e.Args[1]))
		if fe.jsonDecodes(pt.Elem(), e.Args[1]) {
			return "(" + setS + setP + "$rt.jsonDecode(" + s + ", " + fe.desc(pt) + ", " + p + ", true))", true
		}
		r := fe.declareName("$j")
		fe.temps = append(fe.temps, r)
		call := fe.mark(e) + fe.expr(e.Fun) + "(" + data + ", " + fe.convert(p, pt, anyT) + ")"
		return "(" + setS + setP + "(" + r + " = $rt.jsonDecode(" + s + ", " + fe.desc(pt) + ", " + p + ", false)) !== undefined ? " + r + " : " + call + ")", true
	}
	if !isConv {
		return "", false
	}
	setV, v := temp(fe.valueOf(e.Args[1], anyT))
	call := fe.mark(e) + fe.expr(e.Fun) + "(" + data + ", " + v + ")"
	return "(" + setS + setV + "$rt.jsonUnmarshalString(" + s + ", " + v + ") ? null : " + call + ")", true
}

// isStringToBytes reports whether conv converts a string to a []byte.
func (fe *funcEmitter) isStringToBytes(conv *ast.CallExpr) bool {
	tv, ok := fe.info.Types[ast.Unparen(conv.Fun)]
	if !ok || !tv.IsType() || !isByteSlice(tv.Type.Underlying()) {
		return false
	}
	b, ok := fe.info.TypeOf(conv.Args[0]).Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// jsonDecodes reports whether the runtime decodes every JSON value into
// *arg, of type *t, itself (jsonDecode in runtime/src/json.ts, which must
// agree): t is of the types the runtime decodes (fullPlain), and no
// interface in *arg can hold a pointer, which Go decodes into. That is so
// if t has no interfaces where v1 merges (mergesIfaces), or if arg is &v
// for a local v whose interfaces only json.Unmarshal stores into (it stores
// no pointers): v is declared without a value, has its interfaces in its
// own memory (behind no pointer or slice) and every other use of v reads
// it.
func (fe *funcEmitter) jsonDecodes(t types.Type, arg ast.Expr) bool {
	if !jsonFullPlain(t, map[types.Type]bool{}) {
		return false
	}
	switch jsonMergesIfaces(t, false, map[types.Type]bool{}) {
	case ifaceNone:
		return true
	case ifaceShared:
		return false
	}
	u, ok := ast.Unparen(arg).(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return false
	}
	id, ok := ast.Unparen(u.X).(*ast.Ident)
	if !ok {
		return false
	}
	v, ok := fe.info.Uses[id].(*types.Var)
	return ok && fe.pe.freshIfaces[v]
}

// jsonFullPlain mirrors the runtime's fullPlain (with deepPlain and
// structFields).
func jsonFullPlain(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return true
	}
	seen[t] = true
	switch types.Unalias(t).(type) {
	case *types.TypeParam:
		return false
	case *types.Named:
		if types.NewMethodSet(t).Len() > 0 {
			return false
		}
		if !types.IsInterface(t) {
			if _, ok := t.Underlying().(*types.Pointer); !ok && types.NewMethodSet(types.NewPointer(t)).Len() > 0 {
				return false
			}
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool, types.String, types.Float64,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return true
		}
	case *types.Interface:
		return u.Empty()
	case *types.Struct:
		fs, ok := jsonDecFields(u)
		if !ok {
			return false
		}
		for _, f := range fs {
			if !jsonFullPlain(f.Type(), seen) {
				return false
			}
		}
		return true
	case *types.Map:
		k, ok := u.Key().Underlying().(*types.Basic)
		return ok && k.Kind() == types.String && jsonFullPlain(u.Key(), seen) && jsonFullPlain(u.Elem(), seen)
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			return false
		}
		return jsonFullPlain(u.Elem(), seen)
	case *types.Pointer:
		return jsonFullPlain(u.Elem(), seen)
	}
	return false
}

var jsonTagRE = regexp.MustCompile(`(?:^|\s)json:"([^"\\]*)"`)

// jsonDecFields mirrors the runtime's structFields, for the fields the
// runtime decodes itself (with ASCII names): the struct's JSON fields, or
// false.
func jsonDecFields(st *types.Struct) ([]*types.Var, bool) {
	var fs []*types.Var
	seen := map[string]bool{}
	for i := 0; i < st.NumFields(); i++ {
		f, tag := st.Field(i), st.Tag(i)
		if f.Embedded() || strings.Contains(tag, `\`) {
			return nil, false
		}
		if !f.Exported() {
			continue
		}
		name := f.Name()
		if m := jsonTagRE.FindStringSubmatch(tag); m != nil {
			parts := strings.Split(m[1], ",")
			if parts[0] == "-" && len(parts) == 1 {
				continue
			}
			if parts[0] != "" {
				if !jsonNameRE.MatchString(parts[0]) {
					return nil, false
				}
				name = parts[0]
			}
			for _, o := range parts[1:] {
				if o != "omitempty" {
					return nil, false
				}
			}
		}
		for _, c := range []byte(name) {
			if c >= 0x80 {
				return nil, false
			}
		}
		if seen[strings.ToLower(name)] {
			return nil, false
		}
		seen[strings.ToLower(name)] = true
		fs = append(fs, f)
	}
	return fs, true
}

// Where a type has interfaces that v1 merges into (see jsonDecodes).
const (
	ifaceNone   = iota
	ifaceOwn    // in the value's own memory: it, or its fields
	ifaceShared // behind a pointer or in a slice's array
)

// jsonMergesIfaces mirrors the runtime's mergesIfaces, telling apart
// interfaces behind pointers and slices (shared, set).
func jsonMergesIfaces(t types.Type, shared bool, seen map[types.Type]bool) int {
	if seen[t] {
		return ifaceNone
	}
	seen[t] = true
	defer delete(seen, t)
	in := ifaceOwn
	if shared {
		in = ifaceShared
	}
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return in
	case *types.Struct:
		fs, _ := jsonDecFields(u)
		r := ifaceNone
		for _, f := range fs {
			r = max(r, jsonMergesIfaces(f.Type(), shared, seen))
		}
		return r
	case *types.Slice:
		return jsonMergesIfaces(u.Elem(), true, seen)
	case *types.Pointer:
		return jsonMergesIfaces(u.Elem(), true, seen)
	}
	return ifaceNone
}

// freshIfaceVars returns the locals of body that jsonDecodes can tell
// json.Unmarshal stores the only values of their interfaces into: declared
// without values, and used only as &v for json.Unmarshal's second argument
// or read.
func freshIfaceVars(info *types.Info, body *ast.BlockStmt) map[*types.Var]bool {
	if body == nil {
		return nil
	}
	vars := map[*types.Var]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok && len(vs.Values) == 0 {
			for _, id := range vs.Names {
				if v, ok := info.Defs[id].(*types.Var); ok && jsonMergesIfaces(v.Type(), false, map[types.Type]bool{}) == ifaceOwn {
					vars[v] = true
				}
			}
		}
		return true
	})
	if len(vars) == 0 {
		return nil
	}
	// root returns the variable e is a part of, if e is one's field.
	root := func(e ast.Expr) *types.Var {
		for {
			switch x := ast.Unparen(e).(type) {
			case *ast.SelectorExpr:
				if sel := info.Selections[x]; sel == nil || sel.Kind() != types.FieldVal || sel.Indirect() {
					return nil
				}
				e = x.X
				continue
			case *ast.Ident:
				v, _ := info.Uses[x].(*types.Var)
				return v
			}
			return nil
		}
	}
	allowed := map[*ast.UnaryExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if len(n.Args) == 2 && isJSONFunc(info, n, "Unmarshal") {
				if u, ok := ast.Unparen(n.Args[1]).(*ast.UnaryExpr); ok && u.Op == token.AND {
					if _, ok := ast.Unparen(u.X).(*ast.Ident); ok {
						allowed[u] = true
					}
				}
			}
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				delete(vars, root(l))
			}
		case *ast.IncDecStmt:
			delete(vars, root(n.X))
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				for _, x := range []ast.Expr{n.Key, n.Value} {
					if x != nil {
						delete(vars, root(x))
					}
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND && !allowed[n] {
				delete(vars, root(n.X))
			}
		case *ast.SelectorExpr:
			// A method with a pointer receiver called on v (or a field)
			// takes its address.
			if sel := info.Selections[n]; sel != nil && sel.Kind() != types.FieldVal {
				if _, ptr := sel.Obj().(*types.Func).Signature().Recv().Type().(*types.Pointer); ptr {
					delete(vars, root(n.X))
				}
			}
		}
		return true
	})
	return vars
}
