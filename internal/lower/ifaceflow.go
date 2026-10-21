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
// methods are called like an interface's), and, in a program that links
// package reflect, the types reachable from those through fields, elements
// and pointers. Without reflect only Go code puts a value in an interface:
// a chan time.Time stored in an interface value does not store a Time.
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
	ts := p.newIfaceTypes(func(t types.Type, fn *types.Func) {
		p.ifaceImpls[fn.Name()] = append(p.ifaceImpls[fn.Name()], ifaceImpl{t, fn})
	})
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, inst := range info.Instances {
			ts.addInst(inst)
		}
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body != nil {
						fn := info.Defs[d.Name].(*types.Func)
						p.findConversions(info, d.Body, fn.Signature(), ts.conv)
					}
				case *ast.GenDecl:
					p.findConversions(info, d, nil, ts.conv)
				}
			}
		}
	}
}

// ifaceTypes is a set of types stored in interface values, with what
// reflection reaches from them in a program that links package reflect.
// method is called once for each method of each type added.
type ifaceTypes struct {
	msets      *typeutil.MethodSetCache
	reflection bool
	seen       typeutil.Map
	method     func(t types.Type, fn *types.Func)
}

func (p *Program) newIfaceTypes(method func(t types.Type, fn *types.Func)) *ifaceTypes {
	reflection := false
	for _, pkg := range p.Pkgs {
		reflection = reflection || pkg.PkgPath == "reflect"
	}
	return &ifaceTypes{msets: &p.msets, reflection: reflection, method: method}
}

// conv records a conversion from src to dst (see findConversions).
func (s *ifaceTypes) conv(src, dst types.Type) {
	if src != nil && dst != nil && isIface(dst) && !isIface(src) {
		s.add(src)
	}
}

// addInst adds the type arguments of an instantiation.
func (s *ifaceTypes) addInst(inst types.Instance) {
	for i := 0; i < inst.TypeArgs.Len(); i++ {
		if t := inst.TypeArgs.At(i); !isIface(t) {
			s.add(t)
		}
	}
}

func (s *ifaceTypes) add(t types.Type) {
	if t == nil {
		return
	}
	t = types.Unalias(t)
	if s.seen.At(t) != nil {
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
			s.add(u.At(i).Type())
		}
		return
	}
	s.seen.Set(t, true)
	if !isIface(t) {
		ms := s.msets.MethodSet(t)
		for i := 0; i < ms.Len(); i++ {
			s.method(t, ms.At(i).Obj().(*types.Func))
		}
	}
	// Reflection can reach (and take the address of) what a stored value
	// contains.
	if !s.reflection {
		return
	}
	if _, ok := t.(*types.Named); ok {
		s.add(types.NewPointer(t))
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		s.add(u.Elem())
	case *types.Struct:
		// reflect's Interface panics for the value of an unexported field
		// (one not embedded, whose exported fields stay reachable), and its
		// Call for the methods of one, so reflection stores only the
		// values of the others.
		for i := 0; i < u.NumFields(); i++ {
			if f := u.Field(i); f.Exported() || f.Embedded() {
				s.add(f.Type())
				s.add(types.NewPointer(f.Type()))
			}
		}
	case *types.Array:
		s.add(u.Elem())
		s.add(types.NewPointer(u.Elem()))
	case *types.Slice:
		s.add(u.Elem())
		s.add(types.NewPointer(u.Elem()))
	case *types.Map:
		s.add(u.Key())
		s.add(u.Elem())
	case *types.Chan:
		s.add(u.Elem())
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
				case *types.Basic:
					if x.Info()&types.IsInteger != 0 {
						k = info.TypeOf(n.X)
					}
				case *types.Signature: // range over a function: the yield parameters
					if x.Params().Len() == 1 {
						if yield, ok := under(x.Params().At(0).Type()).(*types.Signature); ok {
							if yield.Params().Len() > 0 {
								k = yield.Params().At(0).Type()
							}
							if yield.Params().Len() > 1 {
								v = yield.Params().At(1).Type()
							}
						}
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
		if p.async[impl.fn.Origin()] && p.liveMethod(impl.fn.Origin()) {
			return true
		}
	}
	return false
}

// liveMethod reports whether a call through an interface may reach the
// concrete method fn (refineBlocking): whether reachable code stores its
// type in an interface value and reaches the method.
func (p *Program) liveMethod(fn *types.Func) bool {
	if p.live == nil {
		return true
	}
	return p.live(fn) && p.storedMethods[fn]
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
