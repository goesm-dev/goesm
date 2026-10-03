package lower

import (
	"fmt"
	"go/ast"
	"go/types"
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
	generic := named.TypeParams().Len() > 0
	if isStruct {
		pe.emitStructClass(name, st, named, generic)
		pe.export(name, name)
	}
	pe.export(name+"$type", name+"$type")
	pkgPath := jsString(pe.pkg.PkgPath)
	ctor := "undefined"
	if isStruct {
		ctor = name
	}

	if !generic {
		pe.phase1.ln("%sconst %s$type: $rt.Type = $rt.named(%s, %s);", pe.tab.mark(tn.Pos()), name, pkgPath, jsString(tn.Name()))
		var under string
		if isStruct {
			under = pe.structDesc(st, name, tpScope{})
		} else {
			under = pe.typeDesc(named.Underlying(), tpScope{})
		}
		pe.phase2.ln("$rt.setUnderlying(%s$type, %s, %s);", name, under, ctor)
		pe.methodTables(pe.phase2, name+"$type", named, tpScope{})
		return
	}

	tp := tpScope{inline: true, names: map[*types.TypeParam]string{}}
	var params []string
	for i := 0; i < named.TypeParams().Len(); i++ {
		p := named.TypeParams().At(i)
		n := "$T_" + p.Obj().Name()
		tp.names[p] = n
		params = append(params, n+": $rt.Type")
	}
	w := pe.phase1
	w.ln("%sconst %s$type = $rt.generic(%s, %s, (t: $rt.Type, %s) => {", pe.tab.mark(tn.Pos()), name, pkgPath, jsString(tn.Name()), strings.Join(params, ", "))
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
	w.ln("});")
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
		ms := types.NewMethodSet(T)
		var entries []string
		for i := 0; i < ms.Len(); i++ {
			sel := ms.At(i)
			fn := sel.Obj().(*types.Func)
			if fn.Signature().TypeParams().Len() > 0 {
				continue // generic methods cannot satisfy interfaces
			}
			mtp := tp
			if rtp := fn.Origin().Signature().RecvTypeParams(); rtp != nil {
				for j := 0; j < rtp.Len() && j < named.TypeParams().Len(); j++ {
					mtp = mtp.with(rtp.At(j), tp.names[named.TypeParams().At(j)])
				}
			}
			s := fn.Signature()
			sig := types.NewSignatureType(nil, nil, nil, s.Params(), s.Results(), s.Variadic())
			entries = append(entries, fmt.Sprintf("%s: [%s, %s]", jsString(methodKey(fn)), pe.methodWrapper(T, sel, tp), pe.typeDesc(sig, mtp)))
		}
		if len(entries) > 0 {
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

func derefType(t types.Type) (types.Type, bool) {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		return p.Elem(), true
	}
	return t, false
}

// methodWrapper returns a JS function (recv, ...args) invoking method sel
// on a receiver of type T, following embedded fields and adjusting the
// receiver between value and pointer forms.
func (pe *pkgEmitter) methodWrapper(T types.Type, sel *types.Selection, tp tpScope) string {
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
	recvT := fn.Signature().Recv().Type()
	if isIface(recvT) {
		return fmt.Sprintf("(r: any, ...a: any[]) => $rt.icall(%s, %s, ...a)", recv, jsString(methodKey(fn)))
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
	args := pe.recvTypeArgs(base, tp)
	return fmt.Sprintf("(r: any, ...a: any[]) => %s(%s%s, ...a)", pe.methodFuncName(fn), args, recv)
}

// recvTypeArgs returns "d1, d2, " for the type arguments of a generic
// receiver type (empty otherwise).
func (pe *pkgEmitter) recvTypeArgs(recvBase types.Type, tp tpScope) string {
	named, ok := types.Unalias(recvBase).(*types.Named)
	if !ok || named.TypeArgs().Len() == 0 {
		return ""
	}
	var b strings.Builder
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
	if generic {
		var ps []string
		for i := 0; i < named.TypeParams().Len(); i++ {
			ps = append(ps, jsName(named.TypeParams().At(i).Obj().Name()))
		}
		tparams = "<" + strings.Join(ps, ", ") + ">"
	}
	w.ln("class %s%s {", name, tparams)
	w.indent++
	var params, assigns, clones, sets []string
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		prop := fieldProp(s, i)
		ft := pe.tsType(f.Type(), tp)
		w.ln("%s: %s;", prop, ft)
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
				if n, ok := types.Unalias(f.Type()).(*types.Named); ok && n.TypeArgs().Len() > 0 {
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
	w.ln("constructor(%s) { %s }", strings.Join(params, ", "), strings.Join(assigns, " "))
	w.ln("$clone($t?: $rt.Type): %s { return new %s(%s); }", name, name, strings.Join(clones, ", "))
	w.ln("$set(o: %s, $t?: $rt.Type): void { %s }", name, strings.Join(sets, " "))
	// Exported methods are also reachable as JS methods for convenience.
	if named != nil && !generic {
		ms := types.NewMethodSet(types.NewPointer(named))
		for i := 0; i < ms.Len(); i++ {
			sel := ms.At(i)
			fn := sel.Obj().(*types.Func)
			if !fn.Exported() || len(sel.Index()) != 1 || fn.Signature().TypeParams().Len() > 0 || isFieldName(s, fn.Name()) {
				continue
			}
			w.ln("%s(...a: any[]): any { return %s(this, ...a); }", jsPropName(fn.Name()), pe.methodFuncName(fn))
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
	switch {
	case fd.Recv != nil:
		name = pe.funcDeclName(fd, fn)
		pe.export(name, name)
	case fd.Name.Name == "init":
		name = pe.fresh("init")
		pe.inits = append(pe.inits, name)
		pe.initObjs = append(pe.initObjs, fn)
	default:
		name = pe.funcDeclName(fd, fn)
		if fn.Exported() {
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
		w.ln("%sfunction %s(...a: any[]): any { return $natives.%s(...a); }", pe.tab.mark(fd.Pos()), name, goesmruntime.NativeName(fn.FullName()))
		return
	}
	fe := pe.newFuncEmitter(w, sig)
	fe.file = file
	fe.async = pe.prog.IsAsync(fn)

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
	fe.funcBody(fd.Recv, fd.Type, fd.Body, sig)
	w.indent--
	w.ln("}")
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
		pkg = fn.Pkg().Path() + "."
	}
	return fmt.Sprintf("value method %s%s.%s called using nil *%s pointer", pkg, name, fn.Name(), name)
}
