package lower

import "go/types"

// Method tables and dead code. A type's method table (addMethods) is how
// methods are called dynamically: through an interface, a type parameter, or
// reflection. Bundlers cannot drop entries of a table, so a table listing
// every method keeps every method of every type that is used at all, with
// everything they call: time.Time's 47 methods take Format, the JSON and
// binary encoders and the zone loading into any program that uses time.
//
// As Go's linker does, the tables list only the methods that can be called
// dynamically: those whose name and signature are those of a method of an
// interface type of the program (the universe's error, the runtime's
// String() string, and every interface type written in the packages, which
// includes the constraints of type parameters). Methods called statically
// are referenced by their functions and kept by the bundler as usual. A
// program that enumerates methods by reflection (reflect's Method,
// MethodByName and NumMethod of Type and Value, used outside reflect, or a
// method of the program's own interface with the signature of Method or
// MethodByName) keeps every exported method, as Go's linker does.

// findDynMethods collects the interface methods of the program.
func (p *Program) findDynMethods() {
	p.ifaceMethods = map[string][]ifaceMethod{}
	seen := map[*types.Interface]bool{}
	addIface := func(it *types.Interface) {
		if seen[it] {
			return
		}
		seen[it] = true
		for i := 0; i < it.NumMethods(); i++ {
			m := it.Method(i)
			// A signature with type parameters (of a generic interface
			// instantiated in generic code) matches by name.
			p.ifaceMethods[m.Name()] = append(p.ifaceMethods[m.Name()], ifaceMethod{m, stripRecv(m.Signature()), hasTypeParam(m.Signature())})
		}
	}
	addIface(types.Universe.Lookup("error").Type().Underlying().(*types.Interface))
	// The runtime prints a panic value's String() (runtime/src/panic.ts),
	// like the gc runtime's stringer interface.
	str := types.NewFunc(0, nil, "String", types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewParam(0, nil, "", types.Typ[types.String])), false))
	p.ifaceMethods["String"] = append(p.ifaceMethods["String"], ifaceMethod{str, str.Signature(), false})

	reflectEnum := map[string]bool{
		"(reflect.Type).Method": true, "(reflect.Type).MethodByName": true, "(reflect.Type).NumMethod": true,
		"(reflect.Value).Method": true, "(reflect.Value).MethodByName": true, "(reflect.Value).NumMethod": true,
	}
	for _, pkg := range p.Pkgs {
		info := pkg.TypesInfo
		for _, tv := range info.Types {
			if it, ok := tv.Type.Underlying().(*types.Interface); ok {
				addIface(it)
			}
		}
		for _, obj := range info.Defs {
			if obj == nil {
				continue
			}
			if it, ok := obj.Type().Underlying().(*types.Interface); ok {
				addIface(it)
			}
		}
		if pkg.PkgPath == "reflect" || p.allMethods {
			continue
		}
		for _, obj := range info.Uses {
			if fn, ok := obj.(*types.Func); ok && (reflectEnum[fn.FullName()] || reflectLike(fn)) {
				p.allMethods = true
				break
			}
		}
	}
}

// reflectLike reports whether fn is a method named Method or MethodByName
// returning a reflect.Method, as an interface of the program's own can
// declare to call reflect.Type's (Go's compiler treats these calls the same).
func reflectLike(fn *types.Func) bool {
	if fn.Name() != "Method" && fn.Name() != "MethodByName" {
		return false
	}
	res := fn.Signature().Results()
	if res.Len() == 0 {
		return false
	}
	n, ok := types.Unalias(res.At(0).Type()).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "reflect" && (n.Obj().Name() == "Method" || n.Obj().Name() == "Value")
}

// DynMethod reports whether method fn (a method of a concrete type, possibly
// promoted) belongs in method tables.
func (p *Program) DynMethod(fn *types.Func) bool {
	fn = fn.Origin()
	if p.allMethods && fn.Exported() {
		return true
	}
	sig := fn.Signature()
	generic := sig.RecvTypeParams().Len() > 0
	own := stripRecv(sig)
	for _, m := range p.ifaceMethods[fn.Name()] {
		if !fn.Exported() && m.fn.Pkg() != fn.Pkg() {
			continue
		}
		// The methods of a generic type match by name.
		if generic || m.loose || types.Identical(m.sig, own) {
			return true
		}
	}
	return false
}

type ifaceMethod struct {
	fn    *types.Func
	sig   *types.Signature // without the receiver
	loose bool             // the signature has type parameters
}

func stripRecv(s *types.Signature) *types.Signature {
	return types.NewSignatureType(nil, nil, nil, s.Params(), s.Results(), s.Variadic())
}
