package lower

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// Interface method calls in the blocking analysis. A call x.M() through an
// interface may block only if some concrete type that can be stored in an
// interface value implements x's interface with a method M that blocks. The
// types that can be stored in interface values are those converted to an
// interface type somewhere in the program (assignment, argument, return,
// composite literal element, send, map key, explicit conversion, append,
// panic), the type arguments of every instantiation (a type parameter's
// methods are called like an interface's), and, for reflection, the types
// reachable from those through fields, elements and pointers.
//
// Matching only by method name would make every io.Writer.Write call async
// as soon as the program links io.PipeWriter, whose Write blocks on a
// channel; fmt.Println would then be async everywhere.

// ifaceImpl is a method of a type stored in interface values.
type ifaceImpl struct {
	typ types.Type
	fn  *types.Func // the concrete method, or an interface's for an embedded interface
}

// findIfaceTypes collects the types stored in interface values and indexes
// their methods by name.
func (p *Program) findIfaceTypes() {
	var seen typeutil.Map
	var add func(t types.Type)
	add = func(t types.Type) {
		if t == nil {
			return
		}
		t = types.Unalias(t)
		if seen.At(t) != nil {
			return
		}
		switch u := t.(type) {
		case *types.TypeParam:
			return // covered by the instantiations' type arguments
		case *types.Basic:
			if u.Info()&types.IsUntyped != 0 {
				if u.Kind() == types.UntypedNil {
					return
				}
				t = types.Default(t)
			}
		case *types.Tuple:
			for i := 0; i < u.Len(); i++ {
				add(u.At(i).Type())
			}
			return
		}
		seen.Set(t, true)
		if !isIface(t) {
			ms := p.msets.MethodSet(t)
			for i := 0; i < ms.Len(); i++ {
				fn := ms.At(i).Obj().(*types.Func)
				p.ifaceImpls[fn.Name()] = append(p.ifaceImpls[fn.Name()], ifaceImpl{t, fn})
			}
		}
		// Reflection can reach (and take the address of) what a stored value
		// contains.
		if _, ok := t.(*types.Named); ok {
			add(types.NewPointer(t))
		}
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			add(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				add(u.Field(i).Type())
				add(types.NewPointer(u.Field(i).Type()))
			}
		case *types.Array:
			add(u.Elem())
			add(types.NewPointer(u.Elem()))
		case *types.Slice:
			add(u.Elem())
			add(types.NewPointer(u.Elem()))
		case *types.Map:
			add(u.Key())
			add(u.Elem())
		case *types.Chan:
			add(u.Elem())
		}
	}
	conv := func(src, dst types.Type) {
		if src != nil && dst != nil && isIface(dst) && !isIface(src) {
			add(src)
		}
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, inst := range info.Instances {
			for i := 0; i < inst.TypeArgs.Len(); i++ {
				if t := inst.TypeArgs.At(i); !isIface(t) {
					add(t)
				}
			}
		}
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body != nil {
						fn := info.Defs[d.Name].(*types.Func)
						p.findConversions(info, d.Body, fn.Signature(), conv)
					}
				case *ast.GenDecl:
					p.findConversions(info, d, nil, conv)
				}
			}
		}
	}
}

// findConversions reports to conv every implicit or explicit conversion in
// n, whose enclosing function has signature sig.
func (p *Program) findConversions(info *types.Info, n ast.Node, sig *types.Signature, conv func(src, dst types.Type)) {
	tuple := func(dst func(i int) types.Type, srcs []ast.Expr, n int) {
		if len(srcs) == 1 && n > 1 {
			if t, ok := info.TypeOf(srcs[0]).(*types.Tuple); ok {
				for i := 0; i < t.Len() && i < n; i++ {
					conv(t.At(i).Type(), dst(i))
				}
			}
			return
		}
		for i, e := range srcs {
			if i < n {
				conv(info.TypeOf(e), dst(i))
			}
		}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			if s, ok := info.TypeOf(n).(*types.Signature); ok {
				p.findConversions(info, n.Body, s, conv)
			}
			return false
		case *ast.AssignStmt:
			if n.Tok == token.ASSIGN || n.Tok == token.DEFINE {
				tuple(func(i int) types.Type { return info.TypeOf(n.Lhs[i]) }, n.Rhs, len(n.Lhs))
			}
		case *ast.ValueSpec:
			if n.Type != nil {
				t := info.TypeOf(n.Type)
				tuple(func(int) types.Type { return t }, n.Values, len(n.Names))
			}
		case *ast.ReturnStmt:
			if sig != nil {
				tuple(func(i int) types.Type { return sig.Results().At(i).Type() }, n.Results, sig.Results().Len())
			}
		case *ast.SendStmt:
			if ch, ok := under(info.TypeOf(n.Chan)).(*types.Chan); ok {
				conv(info.TypeOf(n.Value), ch.Elem())
			}
		case *ast.IndexExpr:
			if m, ok := under(info.TypeOf(n.X)).(*types.Map); ok {
				conv(info.TypeOf(n.Index), m.Key())
			}
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				var k, v types.Type
				switch x := under(info.TypeOf(n.X)).(type) {
				case *types.Map:
					k, v = x.Key(), x.Elem()
				case *types.Chan:
					k = x.Elem()
				case *types.Slice:
					v = x.Elem()
				case *types.Array:
					v = x.Elem()
				case *types.Pointer:
					if a, ok := x.Elem().Underlying().(*types.Array); ok {
						v = a.Elem()
					}
				}
				if n.Key != nil && k != nil {
					conv(k, info.TypeOf(n.Key))
				}
				if n.Value != nil && v != nil {
					conv(v, info.TypeOf(n.Value))
				}
			}
		case *ast.CompositeLit:
			switch t := under(info.TypeOf(n)).(type) {
			case *types.Struct:
				for i, e := range n.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						if fv, ok := info.Uses[kv.Key.(*ast.Ident)].(*types.Var); ok {
							conv(info.TypeOf(kv.Value), fv.Type())
						}
					} else if i < t.NumFields() {
						conv(info.TypeOf(e), t.Field(i).Type())
					}
				}
			case *types.Map:
				for _, e := range n.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						conv(info.TypeOf(kv.Key), t.Key())
						conv(info.TypeOf(kv.Value), t.Elem())
					}
				}
			case *types.Slice, *types.Array:
				elem := t.(interface{ Elem() types.Type }).Elem()
				for _, e := range n.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						e = kv.Value
					}
					conv(info.TypeOf(e), elem)
				}
			}
		case *ast.CallExpr:
			if tv, ok := info.Types[unparen(n.Fun)]; ok && tv.IsType() {
				if len(n.Args) == 1 {
					conv(info.TypeOf(n.Args[0]), tv.Type)
				}
				return true
			}
			if id := identOf(n.Fun); id != nil {
				if b, ok := info.Uses[id].(*types.Builtin); ok {
					switch b.Name() {
					case "append":
						if s, ok := under(info.TypeOf(n.Args[0])).(*types.Slice); ok && !n.Ellipsis.IsValid() {
							for _, a := range n.Args[1:] {
								conv(info.TypeOf(a), s.Elem())
							}
						}
					case "panic":
						conv(info.TypeOf(n.Args[0]), types.Universe.Lookup("any").Type())
					}
					return true
				}
			}
			s, ok := under(info.TypeOf(n.Fun)).(*types.Signature)
			if !ok {
				return true
			}
			params := s.Params()
			param := func(i int) types.Type {
				if s.Variadic() && i >= params.Len()-1 {
					last := params.At(params.Len() - 1).Type()
					if n.Ellipsis.IsValid() {
						return last
					}
					return last.Underlying().(*types.Slice).Elem()
				}
				return params.At(i).Type()
			}
			nargs := params.Len()
			if len(n.Args) == 0 {
				return true
			}
			if s.Variadic() && !n.Ellipsis.IsValid() {
				nargs = len(n.Args)
				if t, ok := info.TypeOf(n.Args[0]).(*types.Tuple); ok && len(n.Args) == 1 {
					nargs = t.Len()
				}
			}
			tuple(param, n.Args, nargs)
		}
		return true
	})
}

// ifaceCall is a call of method fn through a value of interface or type
// parameter type recv.
type ifaceCall struct {
	recv types.Type
	fn   *types.Func
}

// ifaceCallBlocks reports whether calling method m through a value of
// interface (or type parameter) type recv may reach a blocking method.
func (p *Program) ifaceCallBlocks(recv types.Type, m *types.Func, seen map[types.Type]bool) bool {
	if seen[recv] {
		return false
	}
	seen[recv] = true
	iface, _ := recv.Underlying().(*types.Interface)
	generic := iface == nil || hasTypeParam(recv)
	for _, impl := range p.ifaceImpls[m.Name()] {
		if !m.Exported() && impl.fn.Pkg() != m.Pkg() {
			continue
		}
		if !generic && !hasTypeParam(impl.typ) && !p.implements(impl.typ, iface) {
			continue
		}
		if r := impl.fn.Signature().Recv(); r != nil && isIface(r.Type()) {
			// Promoted from an embedded interface.
			if p.ifaceCallBlocks(r.Type(), impl.fn, seen) {
				return true
			}
			continue
		}
		if p.async[impl.fn.Origin()] {
			return true
		}
	}
	return false
}

func (p *Program) implements(t types.Type, iface *types.Interface) bool {
	key := [2]any{t, iface}
	r, ok := p.implCache[key]
	if !ok {
		r = types.Implements(t, iface)
		p.implCache[key] = r
	}
	return r
}
