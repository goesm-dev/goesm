package lower

import (
	"fmt"
	"go/ast"
	"go/types"
	"path"
	"strings"

	"github.com/goesm-dev/goesm/internal/natives"
	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// emitNamedType emits the runtime descriptor (and class, for structs) of a
// defined type, plus its method tables. Named type identity, the method set
// and field metadata are kept at run time: interface dispatch, type
// assertions and (later) reflect depend on them.
func (pe *pkgEmitter) emitNamedType(tn *types.TypeName) {
	named := tn.Type().(*types.Named)
	name := pe.namedTypeName(tn)
	st, isStruct := named.Underlying().(*types.Struct)
	outer := outerTypeParams(tn)
	generic := named.TypeParams().Len() > 0 || len(outer) > 0
	if isStruct {
		pe.emitStructClass(name, st, named, generic)
		pe.export(name, name)
	}
	pe.export(name+"$type", name+"$type")
	pkgPath := jsString(goPkgPath(pe.pkg.Types))
	// Type strings use the package name (yaml.Node for gopkg.in/yaml.v3),
	// which the runtime takes from the path unless told otherwise.
	genericExtra := ""
	if n := pe.pkg.Types.Name(); n != path.Base(goPkgPath(pe.pkg.Types)) {
		genericExtra = ", " + jsString(n)
	}
	ctor := "undefined"
	if isStruct {
		ctor = name
	}

	if !generic {
		// The underlying type and the methods are set up by $rt.flushTypes
		// (in phase2), through a pure expression, so that bundlers drop the
		// type with all its methods if nothing refers to it.
		var under string
		if isStruct {
			under = pe.structDesc(st, name, tpScope{})
		} else {
			under = pe.typeDesc(named.Underlying(), tpScope{})
		}
		w := pe.phase1
		pkgName := ""
		if n := pe.pkg.Types.Name(); n != path.Base(goPkgPath(pe.pkg.Types)) {
			pkgName = ", " + jsString(n)
		}
		w.ln("%sconst %s$type: $rt.Type = /* @__PURE__ */ $rt.defined(%s, %s, () => {", pe.tab.mark(tn.Pos()), name, pkgPath, jsString(tn.Name()))
		w.indent++
		self := ""
		if isStruct && ifaceSelf(named) {
			self = ", true"
		}
		w.ln("$rt.setUnderlying(%s$type, %s, %s%s);", name, under, ctor, self)
		pe.methodTables(w, name+"$type", named, tpScope{})
		w.indent--
		w.ln("}%s);", pkgName)
		pe.definesTypes = true
		return
	}

	tp := tpScope{inline: true, names: map[*types.TypeParam]string{}}
	var params []string
	for _, p := range outer {
		// Named apart from the type's own parameters, which may shadow them.
		n := "$F_" + p.Obj().Name()
		tp.names[p] = n
		params = append(params, n+": $rt.Type")
	}
	if g := pe.localGen[tn]; g > 0 {
		if genericExtra == "" {
			genericExtra = ", undefined"
		}
		genericExtra += fmt.Sprintf(", %d, %d", len(outer), g)
	}
	for i := 0; i < named.TypeParams().Len(); i++ {
		p := named.TypeParams().At(i)
		n := "$T_" + p.Obj().Name()
		tp.names[p] = n
		params = append(params, n+": $rt.Type")
	}
	w := pe.phase1
	w.ln("%sconst %s$type = /* @__PURE__ */ $rt.generic(%s, %s, (t: $rt.Type, %s) => {", pe.tab.mark(tn.Pos()), name, pkgPath, jsString(tn.Name()), strings.Join(params, ", "))
	w.indent++
	var under string
	if isStruct {
		under = pe.structDesc(st, name, tp)
	} else {
		under = pe.typeDesc(named.Underlying(), tp)
	}
	w.ln("$rt.setUnderlying(t, %s, %s);", under, ctor)
	pe.methodTables(w, "t", named, tp)
	w.indent--
	w.ln("}%s);", genericExtra)
}

// methodTables registers the method sets of T and *T on the descriptor.
func (pe *pkgEmitter) methodTables(w *writer, desc string, named *types.Named, tp tpScope) {
	if isIface(named) {
		return
	}
	for _, ptr := range []bool{false, true} {
		var T types.Type = named
		target := desc
		if ptr {
			T = types.NewPointer(named)
			target = "$rt.ptrTo(" + desc + ")"
		}
		if entries := pe.methodEntries(T, named, tp); len(entries) > 0 {
			w.ln("$rt.addMethods(%s, {", target)
			w.indent++
			for _, e := range entries {
				w.ln("%s,", e)
			}
			w.indent--
			w.ln("});")
		}
	}
}

// methodEntries returns the addMethods entries for the method set of T.
// named is T's defined type (nil for unnamed struct types, whose methods
// are promoted from embedded fields).
func (pe *pkgEmitter) methodEntries(T types.Type, named *types.Named, tp tpScope) []string {
	ms := types.NewMethodSet(T)
	var entries []string
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		fn := sel.Obj().(*types.Func)
		if fn.Signature().TypeParams().Len() > 0 {
			continue // generic methods cannot satisfy interfaces
		}
		mtp := tp
		if rtp := fn.Origin().Signature().RecvTypeParams(); rtp != nil && named != nil {
			for j := 0; j < rtp.Len() && j < named.TypeParams().Len(); j++ {
				mtp = mtp.with(rtp.At(j), tp.names[named.TypeParams().At(j)])
			}
		}
		s := fn.Signature()
		sig := types.NewSignatureType(nil, nil, nil, s.Params(), s.Results(), s.Variadic())
		entries = append(entries, fmt.Sprintf("%s: [%s, %s]", jsString(methodKey(fn)), pe.methodEntry(T, sel, tp), pe.typeDesc(sig, mtp)))
	}
	return entries
}

// methodEntry is the function of method sel in the table of T: the method's
// own function where it takes a receiver of type T as it is, so that a call
// through the table (icall) reaches it directly, and a methodWrapper
// otherwise.
func (pe *pkgEmitter) methodEntry(T types.Type, sel *types.Selection, tp tpScope) string {
	fn := sel.Obj().(*types.Func)
	if len(sel.Index()) == 1 && types.Identical(fn.Signature().Recv().Type(), T) && pe.recvTypeArgs(fn.Signature().Recv().Type(), tp) == "" {
		if named, ok := types.Unalias(T).(*types.Named); !ok || named.TypeParams().Len() == 0 {
			return pe.methodFuncName(fn)
		}
	}
	return pe.methodWrapper(T, sel, tp, "")
}

// promotedMethods reports whether t is an unnamed struct type, or a pointer
// to one, whose method set (promoted from embedded fields) is not empty.
func promotedMethods(t types.Type) bool {
	base := t
	if p, ok := t.(*types.Pointer); ok {
		base = types.Unalias(p.Elem())
	}
	if _, ok := base.(*types.Struct); !ok {
		return false
	}
	return types.NewMethodSet(t).Len() > 0
}

func derefType(t types.Type) (types.Type, bool) {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		return p.Elem(), true
	}
	return t, false
}

// methodWrapper returns a JS function (recv, ...args) invoking method sel
// on a receiver of type T, following embedded fields and adjusting the
// receiver between value and pointer forms.
// methodTargs are the dictionaries of a generic method's own type arguments
// ("d1, d2, "), for a method expression that instantiates it.
func (pe *pkgEmitter) methodWrapper(T types.Type, sel *types.Selection, tp tpScope, methodTargs string) string {
	fn := sel.Obj().(*types.Func)
	path := sel.Index()
	recv := "r"
	parent := ""
	parentProp := ""
	cur := T
	for _, idx := range path[:len(path)-1] {
		base, _ := derefType(cur)
		st := base.Underlying().(*types.Struct)
		parent, parentProp = recv, fieldProp(st, idx)
		recv = recv + "." + parentProp
		cur = st.Field(idx).Type()
	}
	// The wrapper takes the method's parameters one by one: a rest parameter
	// and spread would cost every call through an interface.
	n := fn.Signature().Params().Len()
	var ps, as []string
	for i := 0; i < n; i++ {
		ps = append(ps, fmt.Sprintf(", a%d: any", i))
		as = append(as, fmt.Sprintf("a%d", i))
	}
	params, argList := strings.Join(ps, ""), strings.Join(as, ", ")
	recvT := fn.Signature().Recv().Type()
	if isIface(recvT) {
		return "(r: any" + params + ") => " + icallExpr(recv, jsString(methodKey(fn)), argList, n)
	}
	_, wantPtr := recvT.(*types.Pointer)
	base, havePtr := derefType(cur)
	switch {
	case wantPtr && !havePtr:
		if !isAggregate(base) {
			recv = fmt.Sprintf("$rt.fieldPtr(%s, %s)", parent, jsString(parentProp))
		}
	case !wantPtr && havePtr:
		if len(path) == 1 {
			recv = fmt.Sprintf("$rt.derefMethod(%s, %s)", recv, jsString(panicwrapMsg(fn, base)))
		} else {
			recv = "$rt.deref(" + recv + ")"
		}
		if !isAggregate(base) {
			recv += ".v" // otherwise the object is the pointer
		}
	}
	args := pe.recvTypeArgs(base, tp) + methodTargs
	if argList != "" {
		argList = ", " + argList
	}
	return fmt.Sprintf("(r: any%s) => (%s as any)(%s%s%s)", params, pe.methodFuncName(fn), args, recv, argList)
}

// recvTypeArgs returns "d1, d2, " for the type arguments of a generic
// receiver type (empty otherwise).
func (pe *pkgEmitter) recvTypeArgs(recvBase types.Type, tp tpScope) string {
	named, ok := types.Unalias(recvBase).(*types.Named)
	if !ok {
		return ""
	}
	var b strings.Builder
	if named.TypeArgs().Len() == 0 {
		// The origin of a generic type (its own method tables): the
		// dictionaries are the type's parameters in scope.
		for i := 0; i < named.TypeParams().Len(); i++ {
			b.WriteString(tp.names[named.TypeParams().At(i)])
			b.WriteString(", ")
		}
		return b.String()
	}
	for i := 0; i < named.TypeArgs().Len(); i++ {
		b.WriteString(pe.typeDesc(named.TypeArgs().At(i), tp))
		b.WriteString(", ")
	}
	return b.String()
}

// methodFuncName is the JS function implementing a concrete method.
func (pe *pkgEmitter) methodFuncName(fn *types.Func) string {
	fn = fn.Origin()
	recv := fn.Signature().Recv().Type()
	base, _ := derefType(recv)
	named := types.Unalias(base).(*types.Named).Origin()
	obj := named.Obj()
	if obj.Pkg() == pe.pkg.Types {
		return pe.namedTypeName(obj) + "$" + fn.Name()
	}
	return pe.qualify(obj.Pkg(), jsName(obj.Name())+"$"+fn.Name())
}

// emitStructClass emits the JS class for struct values. Struct values are
// mutable objects with Go copy semantics provided by $clone/$set; the object
// identity doubles as the struct's address.
func (pe *pkgEmitter) emitStructClass(name string, s *types.Struct, named *types.Named, generic bool) {
	w := newWriter(pe.tab)
	tp := tpScope{}
	tparams := ""
	self := name // the class type
	if generic {
		var ps, names []string
		for i := 0; i < named.TypeParams().Len(); i++ {
			p := named.TypeParams().At(i)
			ps = append(ps, jsName(p.Obj().Name()))
			names = append(names, jsName(p.Obj().Name()))
			if tp.ts == nil {
				tp.ts = map[*types.TypeParam]bool{}
			}
			tp.ts[p] = true
		}
		if len(ps) > 0 {
			tparams = "<" + strings.Join(ps, ", ") + ">"
			self = name + "<" + strings.Join(names, ", ") + ">"
		}
	}
	w.ln("class %s%s {", name, tparams)
	w.indent++
	var params, assigns, clones, sets []string
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		prop := fieldProp(s, i)
		ft := pe.tsType(f.Type(), tp)
		// declare: a type only. A class field (prop: T;) would be defined as
		// undefined before the constructor runs, which makes V8 keep every field
		// as a tagged value, boxing each float64 store.
		w.ln("declare %s: %s;", prop, ft)
		params = append(params, fmt.Sprintf("%s: %s", "$"+prop, ft))
		assigns = append(assigns, fmt.Sprintf("this.%s = $%s;", prop, prop))
		src, dst := "this."+prop, "o."+prop
		switch {
		case hasTypeParam(f.Type()):
			ftDesc := fmt.Sprintf("$t!.fields[%d].type", i)
			clones = append(clones, fmt.Sprintf("$rt.copy(%s, %s)", ftDesc, src))
			sets = append(sets, fmt.Sprintf("this.%s = $rt.copy(%s, %s);", prop, ftDesc, dst))
		case isAggregate(f.Type()):
			clones = append(clones, pe.copyExpr(src, f.Type(), tp))
			if _, ok := f.Type().Underlying().(*types.Struct); ok {
				arg := ""
				if isGenericType(f.Type()) {
					arg = ", " + pe.typeDesc(f.Type(), tp)
				}
				sets = append(sets, fmt.Sprintf("this.%s.$set(%s%s);", prop, dst, arg))
			} else {
				sets = append(sets, fmt.Sprintf("$rt.assign(%s, this.%s, %s);", pe.typeDesc(f.Type(), tp), prop, dst))
			}
		default:
			clones = append(clones, src)
			sets = append(sets, fmt.Sprintf("this.%s = %s;", prop, dst))
		}
	}
	if named != nil && ifaceSelf(named) {
		// The value is its own interface value (see $rt.Iface).
		w.ln("declare readonly t: $rt.Type;")
		w.ln("declare readonly v: %s;", self)
		assigns = append(assigns, "this.v = this;")
	}
	w.ln("constructor(%s) { %s }", strings.Join(params, ", "), strings.Join(assigns, " "))
	w.ln("$clone($t?: $rt.Type): %s { return new %s(%s); }", self, name, strings.Join(clones, ", "))
	w.ln("$set(o: %s, $t?: $rt.Type): void { %s }", self, strings.Join(sets, " "))
	// Exported methods are also reachable as JS methods for convenience.
	if named != nil && !generic {
		ms := types.NewMethodSet(types.NewPointer(named))
		for i := 0; i < ms.Len(); i++ {
			sel := ms.At(i)
			fn := sel.Obj().(*types.Func)
			if !fn.Exported() || len(sel.Index()) != 1 || fn.Signature().TypeParams().Len() > 0 || isFieldName(s, fn.Name()) {
				continue
			}
			var params, args []string
			for j := 0; j < fn.Signature().Params().Len(); j++ {
				a := fmt.Sprintf("a%d", j)
				params = append(params, a+": "+pe.tsType(fn.Signature().Params().At(j).Type(), tp))
				args = append(args, a)
			}
			ret := pe.resultTSType(fn.Signature(), tp)
			if pe.prog.IsAsync(fn) {
				ret = "Promise<" + ret + ">"
			}
			w.ln("%s(%s): %s { return %s(%s); }", jsPropName(fn.Name()), strings.Join(params, ", "), ret, pe.methodFuncName(fn), strings.Join(append([]string{"this"}, args...), ", "))
		}
	}
	w.indent--
	w.ln("}")
	pe.classes.append(w)
}

func isFieldName(s *types.Struct, name string) bool {
	for i := 0; i < s.NumFields(); i++ {
		if s.Field(i).Name() == name {
			return true
		}
	}
	return false
}

// funcDeclName is the JS name of a function or method declaration (not init).
func (pe *pkgEmitter) funcDeclName(fd *ast.FuncDecl, fn *types.Func) string {
	if fd.Recv != nil {
		base, _ := derefType(fn.Signature().Recv().Type())
		return pe.namedTypeName(types.Unalias(base).(*types.Named).Origin().Obj()) + "$" + fn.Name()
	}
	return jsName(fn.Name())
}

// emitFuncDecl emits a function or method declaration.
func (pe *pkgEmitter) emitFuncDecl(file *ast.File, fd *ast.FuncDecl) {
	fn := pe.info.Defs[fd.Name].(*types.Func)
	sig := fn.Signature()
	if fd.Name.Name == "_" {
		return
	}
	var name string
	// JS calls the exported functions and methods of the entry package
	// through a wrapper (see exportWrapper).
	wrap := pe.isEntry && fn.Exported() && fd.Body != nil && fd.Name.Name != "init" &&
		sig.TypeParams().Len() == 0 && sig.RecvTypeParams().Len() == 0
	switch {
	case fd.Recv != nil:
		name = pe.funcDeclName(fd, fn)
		if !wrap {
			pe.export(name, name)
		}
	case fd.Name.Name == "init":
		name = pe.fresh("init")
		pe.inits = append(pe.inits, name)
		pe.initObjs = append(pe.initObjs, fn)
	default:
		name = pe.funcDeclName(fd, fn)
		if fn.Exported() && !wrap {
			pe.export(name, fn.Name())
		}
	}
	w := pe.funcs
	defer func() {
		// A lowering bug must surface as a diagnostic at the Go position,
		// not crash the compiler.
		if r := recover(); r != nil {
			pe.errorf(fd.Pos(), "internal error lowering %s: %v", fn.FullName(), r)
		}
	}()
	if sym, ok := pe.prog.linkPulls[fn]; ok && fd.Body == nil {
		// A //go:linkname pull: call the function providing sym.
		deferrable := sig.Results().Len() == 0
		w.ln("%sfunction %s(...a: any[]): any { return $rt.linkCall(%s, a, %v); }", pe.tab.mark(fd.Pos()), name, jsString(sym), deferrable)
		return
	}
	if fd.Body == nil || pe.std && natives.Override(fn.FullName()) {
		// Standard library functions without a Go body (assembly,
		// linkname, goesm replacements) are implemented by the runtime's
		// natives.
		switch {
		case !pe.std:
			pe.errorf(fd.Pos(), "function %s has no Go body (assembly or linkname); goesm implements such functions only for the standard library", fn.FullName())
		case !goesmruntime.HasNative(fn.FullName()):
			pe.errorf(fd.Pos(), "function %s has no Go body and no native implementation", fn.FullName())
		}
		pe.usesNatives = true
		// Type parameters are declared (unused) so calls may pass TS
		// type arguments like any other generic function.
		generics := ""
		if tps := fn.Signature().TypeParams(); tps.Len() > 0 {
			var ns []string
			for i := 0; i < tps.Len(); i++ {
				ns = append(ns, "_"+jsName(tps.At(i).Obj().Name()))
			}
			generics = "<" + strings.Join(ns, ", ") + ">"
		}
		native := goesmruntime.NativeName(fn.FullName())
		if generics != "" || fn.Signature().Recv() != nil {
			// Callers also pass type dictionaries (and a receiver).
			w.ln("%sfunction %s%s(...a: any[]): any { return ($natives.%s as any)(...a); }", pe.tab.mark(fd.Pos()), name, generics, native)
			return
		}
		// A plain function forwards its parameters one by one, which V8
		// inlines (math.Sqrt is Math.sqrt), unlike a rest parameter and spread.
		var ps, as []string
		for i := 0; i < fn.Signature().Params().Len(); i++ {
			ps = append(ps, fmt.Sprintf("a%d: any", i))
			as = append(as, fmt.Sprintf("a%d", i))
		}
		w.ln("%sfunction %s(%s): any { return ($natives.%s as any)(%s); }", pe.tab.mark(fd.Pos()), name, strings.Join(ps, ", "), native, strings.Join(as, ", "))
		return
	}
	fe := pe.newFuncEmitter(w, sig)
	fe.file = file
	fe.recoverTok = fn.Origin().FullName()
	fe.async = pe.prog.IsAsync(fn)
	fe.syncOnly = pe.prog.SyncOnly(fn)

	var params []string
	var tsParams []string
	addTP := func(list *types.TypeParamList) {
		for i := 0; list != nil && i < list.Len(); i++ {
			p := list.At(i)
			n := fe.declareName("$T_" + p.Obj().Name())
			fe.tp = fe.tp.with(p, n)
			params = append(params, n+": $rt.Type")
			tsParams = append(tsParams, jsName(p.Obj().Name()))
		}
	}
	addTP(sig.RecvTypeParams())
	addTP(sig.TypeParams())
	var recvField *ast.FieldList
	if fd.Recv != nil {
		recvField = fd.Recv
	}
	params = append(params, fe.paramList(recvField, sig.Recv())...)
	params = append(params, fe.paramList(fd.Type.Params, nil)...)
	ret := fe.resultTSType(sig)
	asyncKw := ""
	if fe.async {
		asyncKw = "async "
		ret = "Promise<" + ret + ">"
	}
	generics := ""
	if len(tsParams) > 0 {
		generics = "<" + strings.Join(tsParams, ", ") + ">"
	}
	w.ln("%s%sfunction %s%s(%s): %s {", pe.tab.mark(fd.Pos()), asyncKw, name, generics, strings.Join(params, ", "), ret)
	w.indent++
	pe.byteBools, pe.byteBoolMakes = byteBools(pe.info, fd.Body)
	pe.inBounds = inBoundsIndices(pe.info, fd.Body)
	pe.split = split64Vars(pe.info, fd.Body, pe.prog.boxed)
	pe.strBytes, pe.strMarshal = stringBytesVars(pe.info, fd.Body, pe.prog.boxed)
	fe.funcBody(fd.Recv, fd.Type, fd.Body, sig)
	pe.byteBools, pe.byteBoolMakes, pe.inBounds, pe.split = nil, nil, nil, nil
	pe.strBytes, pe.strMarshal = nil, nil
	w.indent--
	w.ln("}")
	if wrap {
		exported := fn.Name()
		if fd.Recv != nil {
			exported = name
		}
		pe.exportWrapper(name, exported, params, ret, fe.async)
	}
}

// exportWrapper emits and exports, as exported, the function JS calls for
// the entry package's function name. Go code dereferences pointers without
// an explicit nil check (nilChecked), so a nil pointer is a JS TypeError
// until something converts it; the wrapper does, so that a panic reaches JS
// as a GoPanic. Go callers call name itself, and V8 inlines name into the
// wrapper.
func (pe *pkgEmitter) exportWrapper(name, exported string, params []string, ret string, async bool) {
	var args []string
	for _, p := range params {
		n, _, _ := strings.Cut(p, ": ")
		args = append(args, n)
	}
	call := fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
	kw := ""
	if async {
		kw, call = "async ", "await "+call
	}
	w := pe.funcs
	js := pe.fresh(name + "$js")
	w.ln("%sfunction %s(%s): %s {", kw, js, strings.Join(params, ", "), ret)
	w.ln("  try {")
	w.ln("    return %s;", call)
	w.ln("  } catch (e) {")
	w.ln("    throw $rt.toPanic(e);")
	w.ln("  }")
	w.ln("}")
	pe.export(js, exported)
	pe.wrappers[name] = js
}

// panicwrapMsg is Go's panic message for a value method called through a nil
// pointer by a method expression or interface method table.
func panicwrapMsg(fn *types.Func, recvBase types.Type) string {
	name := types.TypeString(recvBase, func(*types.Package) string { return "" })
	if n, ok := types.Unalias(recvBase).(*types.Named); ok {
		name = n.Obj().Name()
	}
	pkg := ""
	if fn.Pkg() != nil {
		pkg = goPkgPath(fn.Pkg()) + "."
	}
	return fmt.Sprintf("value method %s%s.%s called using nil *%s pointer", pkg, name, fn.Name(), name)
}
