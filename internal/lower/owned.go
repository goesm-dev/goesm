package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Appending to a slice in place.
//
// A slice value is a Slice object, its header, which the lowering never
// changes: append returns a new header, and assigning a slice copies the
// reference. A loop of x = append(x, v) therefore allocated a header for
// each element, which JavaScriptCore does several times slower than V8.
//
// A local variable that no function literal refers to and whose address is
// not taken can own its header: no other Go variable holds the header
// object it holds. x = append(x, v) then stores v and increases the
// header's length in place when the capacity allows, as Go does to the
// array anyway; nothing can observe the old header and its length. The
// variable owns its header once an append made it, and stops owning it
// whenever its value may be copied somewhere else: at any use of the
// variable but len, cap, indexing, slicing, string conversion, copy, a
// spread argument of append and comparison with nil, and at an assignment
// of another value. Ownership is a boolean local next to the variable, set
// by the appends and cleared by those uses and assignments, so that a
// variable can be passed to a function, returned or stored and keep being
// appended to: the next append after such a use makes a new header again.

// ownedSlices are the local slice variables of a function declaration and
// its function literals that can own their headers.
type ownedSlices struct {
	// vars are the variables, by the function body that declares them,
	// and flags the names of their flags.
	vars  map[*ast.BlockStmt][]*types.Var
	flags map[*types.Var]string
	// appends are the statements x = append(x, ...) of those variables;
	// escapes are the uses after which a variable no longer owns its
	// header, and resets the statements assigning it another value.
	appends map[*ast.AssignStmt]*types.Var
	escapes map[*ast.Ident]*types.Var
	resets  map[ast.Stmt][]*types.Var
}

// findOwnedSlices finds the slice variables of fd that can own their
// headers.
func (pe *pkgEmitter) findOwnedSlices(fd *ast.FuncDecl) *ownedSlices {
	o := &ownedSlices{
		vars:    map[*ast.BlockStmt][]*types.Var{},
		flags:   map[*types.Var]string{},
		appends: map[*ast.AssignStmt]*types.Var{},
		escapes: map[*ast.Ident]*types.Var{},
		resets:  map[ast.Stmt][]*types.Var{},
	}
	pe.ownedIn(o, fd.Recv, fd.Type, fd.Body)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			pe.ownedIn(o, nil, lit.Type, lit.Body)
		}
		return true
	})
	if len(o.appends) == 0 {
		return nil
	}
	return o
}

// ownedCandidate reports whether v is a slice variable whose elements are
// stored as they are (not structs or arrays, which are copied, nor type
// parameters, which may be either).
func (pe *pkgEmitter) ownedCandidate(v *types.Var) bool {
	if v == nil || v.Name() == "_" || pe.prog.boxed[v] || pe.byteBools[v] || isTypeParam(v.Type()) {
		return false
	}
	s, ok := under(v.Type()).(*types.Slice)
	if !ok {
		return false
	}
	e := s.Elem()
	return !isAggregate(e) && !isTypeParam(e) && !hasTypeParam(e)
}

// ownedIn analyzes one function body, without its function literals.
func (pe *pkgEmitter) ownedIn(o *ownedSlices, recv *ast.FieldList, ft *ast.FuncType, body *ast.BlockStmt) {
	info := pe.info
	cands := map[*types.Var]bool{}
	bad := map[*types.Var]bool{}
	var order []*types.Var
	add := func(id *ast.Ident) {
		if v, ok := info.Defs[id].(*types.Var); ok && pe.ownedCandidate(v) && !cands[v] {
			cands[v] = true
			order = append(order, v)
		}
	}
	for _, fl := range []*ast.FieldList{recv, ft.Params, ft.Results} {
		if fl == nil {
			continue
		}
		for _, f := range fl.List {
			for _, id := range f.Names {
				add(id)
			}
		}
	}
	inspectBody := func(f func(ast.Node) bool) {
		ast.Inspect(body, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			return f(n)
		})
	}
	inspectBody(func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, l := range n.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						add(id)
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range n.Names {
				add(id)
			}
		}
		return true
	})
	if len(cands) == 0 {
		return
	}
	varOf := func(e ast.Expr) *types.Var {
		if e == nil {
			return nil
		}
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok {
			return nil
		}
		v, _ := info.Uses[id].(*types.Var)
		if v == nil {
			v, _ = info.Defs[id].(*types.Var)
		}
		if !cands[v] {
			return nil
		}
		return v
	}
	// A function literal referring to a variable shares it.
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(lit.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if v, ok := info.Uses[id].(*types.Var); ok && cands[v] {
					bad[v] = true
				}
			}
			return true
		})
		return false
	})
	// A statement declaring a variable outside loops, in a body without
	// goto, runs before any append to it, once: the flag is still false.
	once := map[ast.Stmt]bool{}
	gotos := false
	depth := 0
	var stack []ast.Node
	inspectBody(func(n ast.Node) bool {
		if n == nil {
			switch stack[len(stack)-1].(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				depth--
			}
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch n := n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			depth++
		case *ast.BranchStmt:
			gotos = gotos || n.Tok == token.GOTO
		case *ast.AssignStmt:
			once[n] = depth == 0 && n.Tok == token.DEFINE
		case *ast.DeclStmt:
			once[n] = depth == 0
		}
		return true
	})
	declares := func(s ast.Stmt) bool { return once[s] && !gotos }
	appends := map[*ast.AssignStmt]*types.Var{}
	escapes := map[*ast.Ident]*types.Var{}
	resets := map[ast.Stmt][]*types.Var{}
	allowed := map[*ast.Ident]bool{} // the uses that keep a header owned
	allow := func(e ast.Expr) {
		if varOf(e) != nil {
			allowed[ast.Unparen(e).(*ast.Ident)] = true
		}
	}
	// Assignments the lowering does not make as statements of their own
	// (per-iteration variables of a for statement, the receives of select
	// cases, range clauses) leave a variable out.
	leaveOut := func(s ast.Stmt) {
		if as, ok := s.(*ast.AssignStmt); ok {
			for _, l := range as.Lhs {
				if v := varOf(l); v != nil {
					bad[v] = true
				}
			}
		}
	}
	inspectBody(func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ForStmt:
			if n.Init != nil {
				leaveOut(n.Init)
			}
		case *ast.CommClause:
			if n.Comm != nil {
				leaveOut(n.Comm)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if v := varOf(e); v != nil {
					bad[v] = true
				}
			}
		case *ast.AssignStmt:
			if n.Tok != token.ASSIGN && n.Tok != token.DEFINE {
				return true
			}
			if len(n.Lhs) != len(n.Rhs) {
				for _, l := range n.Lhs {
					if v := varOf(l); v != nil {
						bad[v] = true
					}
				}
				return true
			}
			for i, l := range n.Lhs {
				v := varOf(l)
				if v == nil {
					continue
				}
				allow(l)
				if call, ok := ast.Unparen(n.Rhs[i]).(*ast.CallExpr); ok && len(n.Lhs) == 1 && n.Tok == token.ASSIGN && len(call.Args) > 1 && varOf(call.Args[0]) == v {
					if b := builtinOf(info, ast.Unparen(call.Fun)); b != nil && b.Name() == "append" {
						appends[n] = v
						allow(call.Args[0])
						continue
					}
				}
				if !declares(n) || info.Defs[l.(*ast.Ident)] == nil {
					resets[n] = append(resets[n], v)
				}
			}
		case *ast.DeclStmt:
			gd, ok := n.Decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				return true
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for _, id := range vs.Names {
					if v := varOf(id); v != nil {
						allow(id)
						if len(vs.Values) != 0 && len(vs.Values) != len(vs.Names) {
							bad[v] = true
						}
						if !declares(n) {
							resets[n] = append(resets[n], v)
						}
					}
				}
			}
		case *ast.CallExpr:
			if tv, ok := info.Types[ast.Unparen(n.Fun)]; ok && tv.IsType() {
				if b, ok := under(tv.Type).(*types.Basic); ok && b.Info()&types.IsString != 0 && len(n.Args) == 1 {
					allow(n.Args[0]) // string(x) copies the elements
				}
				return true
			}
			if b := builtinOf(info, ast.Unparen(n.Fun)); b != nil {
				switch b.Name() {
				case "len", "cap", "copy":
					for _, a := range n.Args {
						allow(a)
					}
				case "append":
					if n.Ellipsis.IsValid() && len(n.Args) == 2 {
						allow(n.Args[1]) // its elements are copied
					}
				}
			}
		case *ast.ReturnStmt:
			// The function ends: no append follows, and no function
			// literal refers to the variable.
			for _, r := range n.Results {
				allow(r)
			}
		case *ast.IndexExpr:
			allow(n.X)
		case *ast.SliceExpr:
			allow(n.X) // a new header
		case *ast.BinaryExpr:
			if (n.Op == token.EQL || n.Op == token.NEQ) && (isNilIdent(info, n.X) || isNilIdent(info, n.Y)) {
				allow(n.X)
				allow(n.Y)
			}
		case *ast.Ident:
			if v := varOf(n); v != nil && info.Uses[n] != nil {
				escapes[n] = v
			}
		}
		return true
	})
	used := map[*types.Var]bool{}
	for _, v := range appends {
		if !bad[v] {
			used[v] = true
		}
	}
	if len(used) == 0 {
		return
	}
	for _, v := range order {
		if used[v] {
			o.vars[body] = append(o.vars[body], v)
		}
	}
	for s, v := range appends {
		if used[v] {
			o.appends[s] = v
		}
	}
	for id, v := range escapes {
		if used[v] && !allowed[id] {
			o.escapes[id] = v
		}
	}
	for s, vs := range resets {
		for _, v := range vs {
			if used[v] {
				o.resets[s] = append(o.resets[s], v)
			}
		}
	}
}

// declareOwned declares the flags of the variables of body that can own
// their headers. The names are those of the whole function declaration, so
// function literals in it do not shadow them.
func (fe *funcEmitter) declareOwned(body *ast.BlockStmt) {
	o := fe.pe.owned
	if o == nil {
		return
	}
	for _, v := range o.vars[body] {
		n := fe.declareName(jsName(v.Name()) + "$own")
		o.flags[v] = n
		fe.w.ln("let %s = false;", n)
	}
}

// ownedEscape returns the read of variable v at id, clearing its flag if
// the use may copy its header.
func (fe *funcEmitter) ownedEscape(id *ast.Ident, v *types.Var, ref string) string {
	o := fe.pe.owned
	if o == nil || o.escapes[id] != v || o.flags[v] == "" {
		return ref
	}
	return "(" + o.flags[v] + " = false, " + ref + ")"
}

// ownedAppend lowers s, x = append(x, ...), of a variable that can own its
// header, reporting whether it did. The flag is the last argument, as an
// element may be computed from a use of x that copies its header.
func (fe *funcEmitter) ownedAppend(s *ast.AssignStmt, m string) bool {
	o := fe.pe.owned
	if o == nil {
		return false
	}
	v, ok := o.appends[s]
	if !ok || o.flags[v] == "" {
		return false
	}
	call := ast.Unparen(s.Rhs[0]).(*ast.CallExpr)
	elem := under(v.Type()).(*types.Slice).Elem()
	if fe.elemTypeArg(elem) != "" {
		return false
	}
	flag, x := o.flags[v], fe.varRef(v)
	rhs, ok := fe.appendInCall(call, elem, x, flag)
	if !ok {
		return false
	}
	fe.w.ln("%s%s = %s%s;", m, x, fe.mark(call), rhs)
	fe.w.ln("%s = true;", flag)
	return true
}

// appendInCall returns the call of the runtime that appends the arguments
// of call, append(x, ...), in place if flag is set.
func (fe *funcEmitter) appendInCall(call *ast.CallExpr, elem types.Type, x, flag string) (string, bool) {
	zero, et := fe.zeroFn(elem), fe.ts(elem)
	if call.Ellipsis.IsValid() {
		src := fe.expr(call.Args[1])
		st := fe.info.TypeOf(call.Args[1])
		if b, ok := under(st).(*types.Basic); ok && b.Info()&types.IsString != 0 {
			return "$rt.appendStringIn(" + x + ", " + src + ", " + flag + ")", true
		}
		if isTypeParam(st) {
			return "", false
		}
		return "$rt.appendSliceIn<" + et + ">(" + x + ", " + src + ", " + zero + ", " + flag + ")", true
	}
	if len(call.Args) == 2 {
		return "$rt.appendIn1<" + et + ">(" + x + ", " + fe.valueOf(call.Args[1], elem) + ", " + zero + ", " + flag + ")", true
	}
	vals := ""
	for i, a := range call.Args[1:] {
		if i > 0 {
			vals += ", "
		}
		vals += fe.valueOf(a, elem)
	}
	return "$rt.appendIn<" + et + ">(" + x + ", [" + vals + "], " + zero + ", " + flag + ")", true
}

// ownedResets clears the flags of the variables and fields s assigns
// another value.
func (fe *funcEmitter) ownedResets(s ast.Stmt) {
	if o := fe.pe.owned; o != nil {
		for _, v := range o.resets[s] {
			if n := o.flags[v]; n != "" {
				fe.w.ln("%s = false;", n)
			}
		}
	}
	for _, sel := range fe.pe.fieldOwnership().resets[s] {
		obj, prop := fe.fieldBase(sel)
		fe.w.ln("%s.%s$own = false;", obj, prop)
	}
}

// Fields that own their headers.
//
// An unexported slice field of a struct type of the package can own its
// header too: x.f = append(x.f, ...) then appends in place while a flag
// property next to the field, f$own, is set. Only the package refers to
// the field, so its uses are known: the same uses as a local's keep the
// header owned, and the others clear the flag of the struct they read it
// from. A copy of the struct value ($clone, $set) gets a header of its own,
// so the original keeps owning its header. The field qualifies when every
// use selects it directly (not promoted) from a local variable or
// parameter of pointer type that no function literal refers to and whose
// address is not taken, so that evaluating the variable again gives the
// same struct, and when the values a self-append appends are computed
// without calls, which could append to the field of the same struct
// through another pointer between the read of the field and the append.

// ownedFields are the fields of a package that can own their headers.
type ownedFields struct {
	fields  map[*types.Var]bool
	appends map[*ast.AssignStmt]*ast.SelectorExpr
	escapes map[*ast.SelectorExpr]bool
	resets  map[ast.Stmt][]*ast.SelectorExpr
}

// fieldOwnership finds the fields of the package that can own their
// headers, once.
func (pe *pkgEmitter) fieldOwnership() *ownedFields {
	if pe.ownFields != nil {
		return pe.ownFields
	}
	o := &ownedFields{
		fields:  map[*types.Var]bool{},
		appends: map[*ast.AssignStmt]*ast.SelectorExpr{},
		escapes: map[*ast.SelectorExpr]bool{},
		resets:  map[ast.Stmt][]*ast.SelectorExpr{},
	}
	pe.ownFields = o
	info := pe.info
	cands := map[*types.Var]bool{}
	scope := pe.pkg.Types.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok || named.TypeParams().Len() > 0 {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if f.Exported() || f.Embedded() || f.Name() == "_" {
				continue
			}
			if s, ok := f.Type().Underlying().(*types.Slice); ok && !isAggregate(s.Elem()) && !hasTypeParam(s.Elem()) {
				cands[f] = true
			}
		}
	}
	if len(cands) == 0 {
		return o
	}
	// The variables function literals refer to, of the functions around
	// them.
	captured := map[*types.Var]bool{}
	for _, f := range pe.pkg.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			ast.Inspect(lit.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if v, ok := info.Uses[id].(*types.Var); ok && (v.Pos() < lit.Pos() || v.Pos() >= lit.End()) {
						captured[v] = true
					}
				}
				return true
			})
			return true
		})
	}
	bad := map[*types.Var]bool{}
	// fieldOf returns the candidate field e selects, marking it bad if the
	// selection does not qualify.
	fieldOf := func(e ast.Expr) (*types.Var, *ast.SelectorExpr) {
		s, ok := ast.Unparen(e).(*ast.SelectorExpr)
		if !ok {
			return nil, nil
		}
		sel, ok := info.Selections[s]
		if !ok || sel.Kind() != types.FieldVal {
			return nil, nil
		}
		f, _ := sel.Obj().(*types.Var)
		if !cands[f] {
			return nil, nil
		}
		if len(sel.Index()) != 1 || pe.selBase(s) == nil {
			bad[f] = true
		} else if v := pe.selBase(s); captured[v] || pe.prog.boxed[v] {
			bad[f] = true
		}
		return f, s
	}
	appends := map[*ast.AssignStmt]*ast.SelectorExpr{}
	resets := map[ast.Stmt][]*ast.SelectorExpr{}
	allowed := map[*ast.SelectorExpr]bool{}
	var uses []*ast.SelectorExpr
	allow := func(e ast.Expr) {
		if f, s := fieldOf(e); f != nil {
			allowed[s] = true
		}
	}
	leaveOut := func(s ast.Stmt) {
		if as, ok := s.(*ast.AssignStmt); ok {
			for _, l := range as.Lhs {
				if f, _ := fieldOf(l); f != nil {
					bad[f] = true
				}
			}
		}
	}
	for _, file := range pe.pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ForStmt:
				if n.Init != nil {
					leaveOut(n.Init)
				}
			case *ast.CommClause:
				if n.Comm != nil {
					leaveOut(n.Comm)
				}
			case *ast.RangeStmt:
				for _, e := range []ast.Expr{n.Key, n.Value} {
					if f, _ := fieldOf(e); f != nil {
						bad[f] = true
					}
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					if f, _ := fieldOf(n.X); f != nil {
						bad[f] = true
					}
				}
			case *ast.AssignStmt:
				for i, l := range n.Lhs {
					f, s := fieldOf(l)
					if f == nil {
						continue
					}
					if len(n.Lhs) != 1 || len(n.Rhs) != 1 || n.Tok != token.ASSIGN {
						bad[f] = true
						continue
					}
					allowed[s] = true
					if call, ok := ast.Unparen(n.Rhs[i]).(*ast.CallExpr); ok && len(call.Args) > 1 {
						if b := builtinOf(info, ast.Unparen(call.Fun)); b != nil && b.Name() == "append" {
							if f2, s2 := fieldOf(call.Args[0]); f2 == f && pe.selBase(s2) == pe.selBase(s) && callFree(info, call.Args[1:]) {
								appends[n] = s
								allowed[s2] = true
								continue
							}
						}
					}
					resets[n] = append(resets[n], s)
				}
			case *ast.CallExpr:
				if tv, ok := info.Types[ast.Unparen(n.Fun)]; ok && tv.IsType() {
					if b, ok := under(tv.Type).(*types.Basic); ok && b.Info()&types.IsString != 0 && len(n.Args) == 1 {
						allow(n.Args[0])
					}
					return true
				}
				if b := builtinOf(info, ast.Unparen(n.Fun)); b != nil {
					switch b.Name() {
					case "len", "cap", "copy":
						for _, a := range n.Args {
							allow(a)
						}
					case "append":
						if n.Ellipsis.IsValid() && len(n.Args) == 2 {
							allow(n.Args[1])
						}
					}
				}
			case *ast.IndexExpr:
				allow(n.X)
			case *ast.SliceExpr:
				allow(n.X)
			case *ast.BinaryExpr:
				if n.Op == token.EQL || n.Op == token.NEQ {
					if isNilIdent(info, n.X) || isNilIdent(info, n.Y) {
						allow(n.X)
						allow(n.Y)
					}
				}
			case *ast.SelectorExpr:
				if f, s := fieldOf(n); f != nil {
					uses = append(uses, s)
				}
			}
			return true
		})
	}
	fieldOfSel := func(s *ast.SelectorExpr) *types.Var {
		return info.Selections[s].Obj().(*types.Var)
	}
	used := map[*types.Var]bool{}
	for _, s := range appends {
		if f := fieldOfSel(s); !bad[f] {
			used[f] = true
		}
	}
	for f := range used {
		o.fields[f] = true
	}
	for st, s := range appends {
		if used[fieldOfSel(s)] {
			o.appends[st] = s
		}
	}
	for _, s := range uses {
		if used[fieldOfSel(s)] && !allowed[s] {
			o.escapes[s] = true
		}
	}
	for st, ss := range resets {
		for _, s := range ss {
			if used[fieldOfSel(s)] {
				o.resets[st] = append(o.resets[st], s)
			}
		}
	}
	return o
}

// selBase returns the local variable or parameter of pointer type that s
// selects a field from, or nil.
func (pe *pkgEmitter) selBase(s *ast.SelectorExpr) *types.Var {
	id, ok := ast.Unparen(s.X).(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := pe.info.Uses[id].(*types.Var)
	if !ok || v.Parent() == pe.pkg.Types.Scope() || v.IsField() {
		return nil
	}
	if _, ok := v.Type().Underlying().(*types.Pointer); !ok {
		return nil
	}
	return v
}

// callFree reports whether computing es calls no function: only
// conversions and the builtins len and cap.
func callFree(info *types.Info, es []ast.Expr) bool {
	free := true
	for _, e := range es {
		ast.Inspect(e, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if tv, ok := info.Types[ast.Unparen(n.Fun)]; ok && tv.IsType() {
					return true
				}
				if b := builtinOf(info, ast.Unparen(n.Fun)); b != nil && (b.Name() == "len" || b.Name() == "cap") {
					return true
				}
				free = false
			case *ast.FuncLit:
				free = false
			case *ast.UnaryExpr:
				free = free && n.Op != token.ARROW
			}
			return free
		})
	}
	return free
}

// isNilIdent reports whether e is the predeclared nil.
func isNilIdent(info *types.Info, e ast.Expr) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = info.Uses[id].(*types.Nil)
	return ok
}

// ownedField reports whether field i of struct s owns its header.
func (pe *pkgEmitter) ownedField(s *types.Struct, i int) bool {
	return pe.fieldOwnership().fields[s.Field(i)]
}

// fieldEscape returns the read prop of obj at s, clearing the field's flag
// if the use may copy its header.
func (fe *funcEmitter) fieldEscape(s *ast.SelectorExpr, obj, prop string) string {
	if !fe.pe.fieldOwnership().escapes[s] {
		return obj + "." + prop
	}
	return "(" + obj + "." + prop + "$own = false, " + obj + "." + prop + ")"
}

// ownedFieldAppend is ownedAppend for x.f = append(x.f, ...).
func (fe *funcEmitter) ownedFieldAppend(s *ast.AssignStmt, m string) bool {
	sel, ok := fe.pe.fieldOwnership().appends[s]
	if !ok {
		return false
	}
	call := ast.Unparen(s.Rhs[0]).(*ast.CallExpr)
	elem := under(fe.info.TypeOf(sel)).(*types.Slice).Elem()
	if fe.elemTypeArg(elem) != "" {
		return false
	}
	obj, prop := fe.fieldBase(sel)
	x, flag := obj+"."+prop, obj+"."+prop+"$own"
	rhs, ok := fe.appendInCall(call, elem, x, flag)
	if !ok {
		return false
	}
	fe.w.ln("%s%s = %s%s;", m, x, fe.mark(call), rhs)
	fe.w.ln("%s = true;", flag)
	return true
}
