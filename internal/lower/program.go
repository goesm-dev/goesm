// Package lower is goesm's semantic lowering: it walks the typed Go AST
// (syntax from go/parser, semantics from go/types, both run by the go
// toolchain via go/packages) and emits one TypeScript module per Go package.
// Go-specific semantics that JavaScript lacks are expressed as calls into
// @goesm/runtime. The generated TypeScript is an IR for esbuild; it is never
// type-checked and TypeScript's type system makes no Go typing decisions.
package lower

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// Diagnostic is a lowering diagnostic at an original .go position. These are
// goesm limitations ("not yet supported"), never Go compile errors: the
// frontend has already accepted the program.
type Diagnostic struct {
	Pos token.Position
	Msg string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s [goesm lowering]", d.Pos, d.Msg)
}

// Program holds whole-program facts needed while lowering any one package.
type Program struct {
	Fset *token.FileSet
	Pkgs []*packages.Package

	byTypes map[*types.Package]*packages.Package

	// async is the set of functions (declarations by *types.Func, literals by
	// *ast.FuncLit) that may block and are therefore lowered to async
	// functions.
	async map[any]bool
	// boxed are variables that need a Cell because their address is taken
	// (or, for package variables, because another package assigns them).
	boxed map[*types.Var]bool

	units []*unit

	Diags []Diagnostic
}

func (p *Program) errorf(pos token.Pos, format string, args ...any) {
	p.Diags = append(p.Diags, Diagnostic{Pos: p.Fset.Position(pos), Msg: fmt.Sprintf(format, args...)})
}

// NewProgram analyses the loaded packages (dependencies first).
func NewProgram(fset *token.FileSet, pkgs []*packages.Package) *Program {
	p := &Program{
		Fset:    fset,
		Pkgs:    pkgs,
		byTypes: map[*types.Package]*packages.Package{},
		async:   map[any]bool{},
		boxed:   map[*types.Var]bool{},
	}
	for _, pkg := range pkgs {
		p.byTypes[pkg.Types] = pkg
	}
	p.analyzeAddrs()
	p.analyzeBlocking()
	return p
}

func isAggregate(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	}
	return false
}

func unparen(e ast.Expr) ast.Expr { return ast.Unparen(e) }

// analyzeAddrs finds variables whose address is taken explicitly (&x) or
// implicitly (x.M() with a pointer receiver), and package variables assigned
// from other packages (ES module bindings are read-only for importers).
func (p *Program) analyzeAddrs() {
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		varOf := func(e ast.Expr) *types.Var {
			switch e := unparen(e).(type) {
			case *ast.Ident:
				v, _ := info.Uses[e].(*types.Var)
				if v == nil {
					v, _ = info.Defs[e].(*types.Var)
				}
				return v
			case *ast.SelectorExpr:
				if _, ok := info.Selections[e]; !ok {
					v, _ := info.Uses[e.Sel].(*types.Var)
					return v
				}
			}
			return nil
		}
		box := func(v *types.Var) {
			if v != nil && !v.IsField() && !isAggregate(v.Type()) {
				p.boxed[v] = true
			}
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						box(varOf(n.X))
					}
				case *ast.SelectorExpr:
					sel, ok := info.Selections[n]
					if !ok || sel.Kind() != types.MethodVal || len(sel.Index()) != 1 {
						return true
					}
					fn := sel.Obj().(*types.Func)
					recv := fn.Signature().Recv()
					if recv == nil {
						return true
					}
					if _, isPtr := recv.Type().(*types.Pointer); isPtr {
						if _, xPtr := info.TypeOf(n.X).Underlying().(*types.Pointer); !xPtr {
							box(varOf(n.X))
						}
					}
				case *ast.AssignStmt:
					for _, l := range n.Lhs {
						if v := varOf(l); v != nil && v.Pkg() != pkg.Types && v.Parent() == v.Pkg().Scope() {
							box(v)
						}
					}
				case *ast.IncDecStmt:
					if v := varOf(n.X); v != nil && v.Pkg() != pkg.Types && v.Parent() == v.Pkg().Scope() {
						box(v)
					}
				}
				return true
			})
		}
	}
}

// Blocking analysis. A Go function is lowered to an async JS function iff it
// may block: it performs a channel operation or select, calls (or defers) a
// function that may block, or makes a dynamic call (function value or
// interface method) that may reach one. Dynamic calls are resolved
// conservatively by signature / method name over the whole program.
type unit struct {
	key       any // *types.Func or *ast.FuncLit
	sig       *types.Signature
	isMethod  bool
	name      string
	blocking  bool
	callees   []any
	dynSigs   []*types.Signature
	ifaceMeth []string
}

func (p *Program) analyzeBlocking() {
	var units []*unit
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if n.Body == nil {
						return false
					}
					fn := info.Defs[n.Name].(*types.Func)
					units = append(units, p.scanUnit(info, fn, fn.Signature(), n.Recv != nil, fn.Name(), n.Body))
				case *ast.FuncLit:
					sig, _ := info.TypeOf(n).(*types.Signature)
					units = append(units, p.scanUnit(info, n, sig, false, "", n.Body))
				}
				return true
			})
		}
	}
	p.units = units
	for changed := true; changed; {
		changed = false
		for _, u := range units {
			if p.async[u.key] {
				continue
			}
			if p.unitBlocks(u, units) {
				p.async[u.key] = true
				changed = true
			}
		}
	}
}

func (p *Program) unitBlocks(u *unit, units []*unit) bool {
	if u.blocking {
		return true
	}
	for _, c := range u.callees {
		if p.async[c] {
			return true
		}
	}
	for _, s := range u.dynSigs {
		for _, o := range units {
			if p.async[o.key] && o.sig != nil && sigMatch(s, o.sig) {
				return true
			}
		}
	}
	for _, m := range u.ifaceMeth {
		for _, o := range units {
			if p.async[o.key] && o.isMethod && o.name == m {
				return true
			}
		}
	}
	return false
}

func (p *Program) scanUnit(info *types.Info, key any, sig *types.Signature, isMethod bool, name string, body *ast.BlockStmt) *unit {
	u := &unit{key: key, sig: sig, isMethod: isMethod, name: name}
	var inGo map[*ast.CallExpr]bool
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false // separate unit
		case *ast.GoStmt:
			if inGo == nil {
				inGo = map[*ast.CallExpr]bool{}
			}
			inGo[n.Call] = true
		case *ast.SendStmt:
			u.blocking = true
		case *ast.UnaryExpr:
			if n.Op == token.ARROW {
				u.blocking = true
			}
		case *ast.SelectStmt:
			hasDefault := false
			for _, c := range n.Body.List {
				if c.(*ast.CommClause).Comm == nil {
					hasDefault = true
				}
			}
			if !hasDefault {
				u.blocking = true
			}
		case *ast.RangeStmt:
			switch info.TypeOf(n.X).Underlying().(type) {
			case *types.Chan:
				u.blocking = true
			case *types.Signature: // range-over-func calls the iterator
				p.classifyFunc(info, n.X, u)
			}
		case *ast.CallExpr:
			// A `go` call's callee runs on its own goroutine; its arguments
			// are still evaluated here and are visited as children.
			p.classifyCall(info, n, u, inGo[n])
		}
		return true
	})
	return u
}

func (p *Program) classifyCall(info *types.Info, call *ast.CallExpr, u *unit, isGo bool) {
	if isGo {
		return
	}
	p.classifyFunc(info, call.Fun, u)
}

// classifyFunc records a call of the function expression fun in u.
func (p *Program) classifyFunc(info *types.Info, fun ast.Expr, u *unit) {
	callee := fun
	fun = unparen(fun)
	if tv, ok := info.Types[fun]; ok && tv.IsType() {
		return // conversion
	}
	// Strip explicit instantiation.
	switch f := fun.(type) {
	case *ast.IndexExpr:
		if _, ok := info.Instances[identOf(f.X)]; ok {
			fun = f.X
		}
	case *ast.IndexListExpr:
		fun = f.X
	}
	switch f := unparen(fun).(type) {
	case *ast.FuncLit:
		u.callees = append(u.callees, f)
		return
	case *ast.Ident:
		switch obj := info.Uses[f].(type) {
		case *types.Builtin:
			return
		case *types.Func:
			u.callees = append(u.callees, obj.Origin())
			return
		}
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[f]; ok {
			switch sel.Kind() {
			case types.MethodVal:
				fn := sel.Obj().(*types.Func)
				if isIface(fn.Signature().Recv().Type()) {
					u.ifaceMeth = append(u.ifaceMeth, fn.Name())
				} else {
					u.callees = append(u.callees, fn.Origin())
				}
				return
			case types.MethodExpr:
				fn := sel.Obj().(*types.Func)
				if isIface(fn.Signature().Recv().Type()) {
					u.ifaceMeth = append(u.ifaceMeth, fn.Name())
				} else {
					u.callees = append(u.callees, fn.Origin())
				}
				return
			}
		} else if obj, ok := info.Uses[f.Sel].(*types.Func); ok {
			u.callees = append(u.callees, obj.Origin())
			return
		}
	}
	if sig, ok := info.TypeOf(callee).Underlying().(*types.Signature); ok {
		u.dynSigs = append(u.dynSigs, sig)
	}
}

func identOf(e ast.Expr) *ast.Ident {
	switch e := unparen(e).(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil
}

func sigMatch(a, b *types.Signature) bool {
	if a.Params().Len() != b.Params().Len() || a.Results().Len() != b.Results().Len() || a.Variadic() != b.Variadic() {
		return false
	}
	if hasTypeParam(a) || hasTypeParam(b) {
		return true // conservative under type parameters
	}
	return types.Identical(types.NewSignatureType(nil, nil, nil, a.Params(), a.Results(), a.Variadic()),
		types.NewSignatureType(nil, nil, nil, b.Params(), b.Results(), b.Variadic()))
}

// hasTypeParam reports whether t mentions a type parameter.
func hasTypeParam(t types.Type) bool {
	seen := map[types.Type]bool{}
	var walk func(t types.Type) bool
	walk = func(t types.Type) bool {
		if t == nil || seen[t] {
			return false
		}
		seen[t] = true
		switch t := types.Unalias(t).(type) {
		case *types.TypeParam:
			return true
		case *types.Named:
			for i := 0; i < t.TypeArgs().Len(); i++ {
				if walk(t.TypeArgs().At(i)) {
					return true
				}
			}
			return false
		case *types.Pointer:
			return walk(t.Elem())
		case *types.Slice:
			return walk(t.Elem())
		case *types.Array:
			return walk(t.Elem())
		case *types.Chan:
			return walk(t.Elem())
		case *types.Map:
			return walk(t.Key()) || walk(t.Elem())
		case *types.Tuple:
			for i := 0; i < t.Len(); i++ {
				if walk(t.At(i).Type()) {
					return true
				}
			}
		case *types.Signature:
			return walk(t.Params()) || walk(t.Results())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				if walk(t.Field(i).Type()) {
					return true
				}
			}
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				if walk(t.Method(i).Type()) {
					return true
				}
			}
		}
		return false
	}
	return walk(t)
}

// CallBlocks reports whether the call may block (and must be awaited).
func (p *Program) CallBlocks(info *types.Info, call *ast.CallExpr) bool {
	u := &unit{}
	p.classifyCall(info, call, u, false)
	return p.unitBlocks(u, p.units)
}

// RangeBlocks reports whether a range-over-func statement's call of its
// iterator may block (and must be awaited).
func (p *Program) RangeBlocks(info *types.Info, s *ast.RangeStmt) bool {
	u := &unit{}
	p.classifyFunc(info, s.X, u)
	return p.unitBlocks(u, p.units)
}

// LitAsync reports whether a function literal may block.
func (p *Program) LitAsync(lit *ast.FuncLit) bool { return p.async[lit] }

// IsAsync reports whether the function declared by fn may block.
func (p *Program) IsAsync(fn *types.Func) bool { return p.async[fn.Origin()] }

// SortedDiags returns diagnostics in position order.
func (p *Program) SortedDiags() []Diagnostic {
	d := append([]Diagnostic(nil), p.Diags...)
	sort.SliceStable(d, func(i, j int) bool {
		a, b := d[i].Pos, d[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return d
}
