package lower

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// uncopied returns the struct types of the entry package whose values are
// never copied or assigned as a whole: the program only ever has pointers
// to them, and reads and writes their fields. Their classes need no $clone
// or $set, which bundlers cannot drop as they can drop unused functions.
//
// Only the entry package's types qualify, since no other Go package can
// reach them, and only when the program links no reflect, which could copy
// any value it is given a pointer to. A type is copied when an expression
// of its type is used other than by taking its address or selecting a field
// or a pointer method from it; when a variable, parameter or result has its
// type; when it is an element of a slice, array, map or channel type or a
// field of a struct type that is copied; or when it is a type argument.
func (pe *pkgEmitter) uncopied() map[*types.TypeName]bool {
	if pe.noCopy != nil {
		return pe.noCopy
	}
	pe.noCopy = map[*types.TypeName]bool{}
	if !pe.isEntry || pe.std || pe.dep || linksReflect(pe.pkg) {
		return pe.noCopy
	}
	cands := map[*types.TypeName]bool{}
	scope := pe.pkg.Types.Scope()
	for _, name := range scope.Names() {
		if tn, ok := scope.Lookup(name).(*types.TypeName); ok && !tn.IsAlias() {
			if _, ok := tn.Type().Underlying().(*types.Struct); ok {
				cands[tn] = true
			}
		}
	}
	if len(cands) == 0 {
		return pe.noCopy
	}
	copied := map[*types.TypeName]bool{}
	fields := map[*types.TypeName][]*types.TypeName{} // a struct type's struct-typed fields
	cand := func(t types.Type) *types.TypeName {
		if n, ok := types.Unalias(t).(*types.Named); ok && cands[n.Origin().Obj()] {
			return n.Origin().Obj()
		}
		return nil
	}
	// mark marks the candidates t holds by value as copied.
	var mark func(t types.Type, seen map[types.Type]bool)
	mark = func(t types.Type, seen map[types.Type]bool) {
		if seen[t] {
			return
		}
		seen[t] = true
		if tn := cand(t); tn != nil {
			copied[tn] = true
			return
		}
		switch u := types.Unalias(t).(type) {
		case *types.Named:
			if u.TypeArgs().Len() > 0 {
				for i := 0; i < u.TypeArgs().Len(); i++ {
					mark(u.TypeArgs().At(i), seen)
				}
			}
			mark(u.Underlying(), seen)
		case *types.Array:
			mark(u.Elem(), seen)
		case *types.Slice:
			mark(u.Elem(), seen)
		case *types.Map:
			mark(u.Key(), seen)
			mark(u.Elem(), seen)
		case *types.Chan:
			mark(u.Elem(), seen)
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				mark(u.Field(i).Type(), seen)
			}
		case *types.Tuple:
			for i := 0; i < u.Len(); i++ {
				mark(u.At(i).Type(), seen)
			}
		}
	}
	// The element and field types of every type the package spells.
	visit := func(t types.Type) {
		switch u := types.Unalias(t).(type) {
		case *types.Pointer:
			t = u.Elem()
		}
		if tn := cand(t); tn != nil {
			// A candidate's fields are copied when it is.
			st := tn.Type().Underlying().(*types.Struct)
			for i := 0; i < st.NumFields(); i++ {
				f := st.Field(i).Type()
				if ft := cand(f); ft != nil {
					fields[tn] = append(fields[tn], ft)
					continue
				}
				mark(f, map[types.Type]bool{})
			}
			return
		}
		switch u := types.Unalias(t).Underlying().(type) {
		case *types.Struct, *types.Array, *types.Slice, *types.Map, *types.Chan:
			mark(u, map[types.Type]bool{})
		}
	}
	for tn := range cands {
		visit(tn.Type())
	}
	for _, inst := range pe.info.Instances {
		for i := 0; i < inst.TypeArgs.Len(); i++ {
			mark(inst.TypeArgs.At(i), map[types.Type]bool{})
		}
	}
	for id, obj := range pe.info.Defs {
		v, ok := obj.(*types.Var)
		if !ok || id == nil {
			continue
		}
		if v.IsField() {
			continue // as part of their struct
		}
		mark(v.Type(), map[types.Type]bool{})
	}
	for _, f := range pe.pkg.Syntax {
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			e, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			tv, ok := pe.info.Types[e]
			if !ok || tv.IsType() {
				return true
			}
			visit(tv.Type)
			tn := cand(tv.Type)
			if tn == nil || pe.notCopiedBy(stack) {
				return true
			}
			copied[tn] = true
			return true
		})
	}
	for changed := true; changed; {
		changed = false
		for tn, fs := range fields {
			if !copied[tn] {
				continue
			}
			for _, f := range fs {
				if !copied[f] {
					copied[f] = true
					changed = true
				}
			}
		}
	}
	for tn := range cands {
		if !copied[tn] {
			pe.noCopy[tn] = true
		}
	}
	return pe.noCopy
}

// notCopiedBy reports whether the expression at the top of stack, of a
// struct type, is not copied by its parent: it has its address taken, or a
// field or a pointer method selected.
func (pe *pkgEmitter) notCopiedBy(stack []ast.Node) bool {
	e := stack[len(stack)-1]
	i := len(stack) - 2
	for i >= 0 {
		if p, ok := stack[i].(*ast.ParenExpr); ok {
			e = p
			i--
			continue
		}
		break
	}
	if i < 0 {
		return false
	}
	switch p := stack[i].(type) {
	case *ast.UnaryExpr:
		return p.Op == token.AND
	case *ast.SelectorExpr:
		if p.X != e {
			return false
		}
		sel := pe.info.Selections[p]
		if sel == nil {
			return false
		}
		switch sel.Kind() {
		case types.FieldVal:
			return true
		case types.MethodVal:
			_, ptr := sel.Obj().(*types.Func).Signature().Recv().Type().(*types.Pointer)
			return ptr
		}
	}
	return false
}

// linksReflect reports whether pkg imports reflect, directly or not.
func linksReflect(pkg *packages.Package) bool {
	seen := map[*packages.Package]bool{}
	var walk func(p *packages.Package) bool
	walk = func(p *packages.Package) bool {
		if seen[p] {
			return false
		}
		seen[p] = true
		if p.PkgPath == "reflect" {
			return true
		}
		for _, q := range p.Imports {
			if walk(q) {
				return true
			}
		}
		return false
	}
	return walk(pkg)
}
