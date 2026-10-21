package lower

import (
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

// The JS calling ABI: JavaScript calling Go.
//
// JavaScript calls the exported functions and methods of the package a
// build starts from (the entry package) through wrappers that convert the
// arguments from JavaScript and the results to JavaScript, by the Go types
// of the declaration, with the table //goesm:import uses the other way
// round (runtime/src/jsabi.ts): a string is a JS string, a slice an array,
// a struct a plain object. What has no plain JavaScript counterpart passes
// as the Go value itself, a handle JavaScript gives back to Go: a pointer to
// a struct type with methods, a non-empty interface, a channel. A final
// error result is thrown as a GoError; several results are an array.
//
// Go code never calls the wrappers: Go callers of the entry package's
// functions call them directly, and the module's $goesm table lists them.

// wrapperName returns the name of the export wrapper of the function or
// method name, which emitStructClass may need before the function is
// emitted.
func (pe *pkgEmitter) wrapperName(name string) string {
	if js, ok := pe.wrappers[name]; ok {
		return js
	}
	js := pe.fresh(strings.ReplaceAll(name, ".", "$") + "$js")
	pe.wrappers[name] = js
	return js
}

// importedHandles are the struct types of other packages whose pointers
// cross the entry package's JS calling ABI as handles: in the parameters
// and results of its exported functions and of its types' exported methods,
// in the fields and elements of what those carry, and in the methods of the
// handles themselves. Other packages' struct classes have no JS methods
// (emitStructClass), so the entry package gives these theirs.
func (pe *pkgEmitter) importedHandles() []*types.Named {
	var found []*types.Named
	for _, n := range handleTypes(pe.pkg.Types) {
		if n.Obj().Pkg() != pe.pkg.Types {
			found = append(found, n)
		}
	}
	return found
}

// handleTypes are the struct types, of pkg and of other packages, whose
// pointers cross the JS calling ABI of the entry package pkg as handles
// (see importedHandles).
func handleTypes(pkg *types.Package) []*types.Named {
	var found []*types.Named
	seen := map[types.Type]bool{}
	// Pointer types are not canonical: *T in a parameter and in a result
	// are different types.Type values.
	seenNamed := map[*types.Named]bool{}
	var walk func(t types.Type)
	walkSig := func(sig *types.Signature) {
		for i := 0; i < sig.Params().Len(); i++ {
			walk(sig.Params().At(i).Type())
		}
		for i := 0; i < sig.Results().Len(); i++ {
			walk(sig.Results().At(i).Type())
		}
	}
	walk = func(t types.Type) {
		t = types.Unalias(t)
		if seen[t] || isJSValue(t) {
			return
		}
		seen[t] = true
		if isHandleType(t) {
			p, ok := t.Underlying().(*types.Pointer)
			if !ok {
				return
			}
			n, ok := types.Unalias(p.Elem()).(*types.Named)
			if !ok || n.TypeArgs().Len() > 0 || seenNamed[n] {
				return
			}
			seenNamed[n] = true
			found = append(found, n)
			for _, fn := range handleMethods(n) {
				walkSig(fn.Signature())
			}
			return
		}
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			walk(u.Elem())
		case *types.Slice:
			walk(u.Elem())
		case *types.Array:
			walk(u.Elem())
		case *types.Map:
			walk(u.Key())
			walk(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if f := u.Field(i); f.Exported() || promotes(f) {
					walk(f.Type())
				}
			}
		case *types.Signature:
			walkSig(u)
		}
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		switch obj := scope.Lookup(name).(type) {
		case *types.Func:
			if obj.Exported() && obj.Signature().TypeParams().Len() == 0 {
				walkSig(obj.Signature())
			}
		case *types.TypeName:
			if n, ok := obj.Type().(*types.Named); ok && !obj.IsAlias() && n.TypeParams().Len() == 0 {
				walk(types.NewPointer(n))
			}
		}
	}
	return found
}

// anyHandleMethods are the exported methods JavaScript calls on handles that
// reach it inside interface values, or inside the slices, maps and structs
// those hold: when the JS calling ABI of the entry
// package carries an interface type other than error (an `any` result, or
// one of a function it returns), the pointers to struct types with methods
// that the entry package, or another package of the program's own module
// (whose function the entry may forward the value from), stores in
// interface values cross as handles whose type no signature names, often
// instantiations of generic types. The standard library and third-party
// packages are not looked at: their boxed types would keep many methods for
// no caller. Their
// method tables list these methods, with their functions, and the runtime
// gives such a handle its JS methods from its table (withJSMethods in
// runtime/src/jsabi.ts). The methods are keyed by their origin.

func (p *Program) anyHandleMethods() map[*types.Func]bool {
	p.anyHandlesOnce.Do(func() {
		p.anyHandles = map[*types.Func]bool{}
		carries := false
		for _, pkg := range p.Pkgs {
			if p.Entries[pkg.PkgPath] && abiCarriesIface(pkg.Types) {
				carries = true
			}
		}
		if !carries {
			return
		}
		// The type parameters whose values are converted to interface types,
		// directly or behind a pointer, and the type arguments
		// instantiations give them.
		type tparam struct {
			tp  *types.TypeParam
			ptr bool
		}
		type instArg struct {
			tp  *types.TypeParam
			arg types.Type
		}
		boxed := map[tparam]bool{}
		changed := false
		var insts []instArg
		// A method's receiver type parameters stand for its type's, which
		// instantiations name.
		recvTP := map[*types.TypeParam]*types.TypeParam{}
		canon := func(tp *types.TypeParam) *types.TypeParam {
			if t, ok := recvTP[tp]; ok {
				return t
			}
			return tp
		}
		mark := func(k tparam) {
			if !boxed[k] {
				boxed[k] = true
				changed = true
			}
		}
		// visit finds the handles JavaScript gets from a value of type t
		// that goes to JavaScript (out) as goToJS (runtime/src/jsabi.ts)
		// converts it: itself and what its methods return, or the
		// elements, keys and fields it converts, what a pointer that is
		// not a handle points to, and what a function returns; and what a
		// function value takes that goes the other way, from JavaScript
		// to Go, whose arguments Go passes to JavaScript. JavaScript calls
		// methods only on the handles it gets: a handle it passes to Go
		// came from Go before.
		var seen, seenIn typeutil.Map
		var visit func(t types.Type, out bool)
		visit = func(t types.Type, out bool) {
			t = types.Unalias(t)
			s := &seen
			if !out {
				s = &seenIn
			}
			if s.At(t) != nil || isJSValue(t) {
				return
			}
			s.Set(t, true)
			if tp, ok := t.(*types.TypeParam); ok {
				if out {
					mark(tparam{canon(tp), false})
				}
				return
			}
			switch u := t.Underlying().(type) {
			case *types.Pointer:
				e := types.Unalias(u.Elem())
				if tp, ok := e.(*types.TypeParam); ok {
					if out {
						mark(tparam{canon(tp), true})
					}
					return
				}
				if n, ok := e.(*types.Named); ok && isHandleType(t) {
					// JavaScript calls the methods, whose arguments and
					// results convert as well.
					if out {
						for _, fn := range handleMethods(n) {
							p.anyHandles[fn.Origin()] = true
							visit(fn.Signature(), true)
						}
					}
					return
				}
				visit(e, out)
			case *types.Slice:
				visit(u.Elem(), out)
			case *types.Array:
				visit(u.Elem(), out)
			case *types.Map:
				visit(u.Key(), out)
				visit(u.Elem(), out)
			case *types.Struct:
				for i := 0; i < u.NumFields(); i++ {
					if f := u.Field(i); f.Exported() || promotes(f) {
						visit(f.Type(), out)
					}
				}
			case *types.Signature:
				// Called by the side the function goes to.
				for i := 0; i < u.Params().Len(); i++ {
					visit(u.Params().At(i).Type(), !out)
				}
				for i := 0; i < u.Results().Len(); i++ {
					visit(u.Results().At(i).Type(), out)
				}
			}
		}
		for _, pkg := range p.Pkgs {
			if !p.Entries[pkg.PkgPath] && (p.std[pkg] || p.Deps[pkg]) {
				continue
			}
			info := pkg.TypesInfo
			conv := func(src, dst types.Type) {
				if src != nil && dst != nil && isIface(dst) && !isIface(src) {
					visit(src, true)
				}
			}
			for _, f := range pkg.Syntax {
				for _, d := range f.Decls {
					switch d := d.(type) {
					case *ast.FuncDecl:
						if d.Body != nil {
							fn := info.Defs[d.Name].(*types.Func)
							sig := fn.Signature()
							if rtps := sig.RecvTypeParams(); rtps.Len() > 0 {
								base, _ := derefType(sig.Recv().Type())
								tps := types.Unalias(base).(*types.Named).Origin().TypeParams()
								for i := 0; i < rtps.Len() && i < tps.Len(); i++ {
									recvTP[rtps.At(i)] = tps.At(i)
								}
							}
							p.findConversions(info, d.Body, sig, conv)
						}
					case *ast.GenDecl:
						p.findConversions(info, d, nil, conv)
					}
				}
			}
			for id, inst := range info.Instances {
				var tps *types.TypeParamList
				switch obj := info.Uses[id].(type) {
				case *types.Func:
					tps = obj.Origin().Signature().TypeParams()
				case *types.TypeName:
					if n, ok := types.Unalias(obj.Type()).(*types.Named); ok {
						tps = n.Origin().TypeParams()
					}
				}
				for i := 0; tps != nil && i < tps.Len() && i < inst.TypeArgs.Len(); i++ {
					insts = append(insts, instArg{tps.At(i), inst.TypeArgs.At(i)})
				}
			}
		}
		// A generic function (or method) that boxes a value of a type
		// parameter, as func box[T any](v T) any { return v } does, boxes
		// the type arguments of its instantiations, through those of the
		// generic functions that pass their own type parameters on.
		for changed {
			changed = false
			for _, ia := range insts {
				if boxed[tparam{ia.tp, false}] {
					visit(ia.arg, true)
				}
				if boxed[tparam{ia.tp, true}] {
					visit(types.NewPointer(ia.arg), true)
				}
			}
		}
	})
	return p.anyHandles
}

// abiCarriesIface reports whether the JS calling ABI of pkg, its exported
// functions and the exported methods of its types, carries a value of an
// interface type other than error, which can hold a handle of any type.
func abiCarriesIface(pkg *types.Package) bool {
	errType := types.Universe.Lookup("error").Type()
	seen := map[types.Type]bool{}
	var walk func(t types.Type) bool
	walkSig := func(sig *types.Signature) bool {
		for i := 0; i < sig.Params().Len(); i++ {
			if walk(sig.Params().At(i).Type()) {
				return true
			}
		}
		for i := 0; i < sig.Results().Len(); i++ {
			if walk(sig.Results().At(i).Type()) {
				return true
			}
		}
		return false
	}
	walk = func(t types.Type) bool {
		t = types.Unalias(t)
		if seen[t] || isJSValue(t) || types.Identical(t, errType) {
			return false
		}
		seen[t] = true
		switch u := t.Underlying().(type) {
		case *types.Interface:
			return true
		case *types.Pointer:
			// JavaScript calls the methods of a handle, whatever its
			// type's name.
			if n, ok := types.Unalias(u.Elem()).(*types.Named); ok && isHandleType(t) {
				for _, fn := range handleMethods(n) {
					if walkSig(fn.Signature()) {
						return true
					}
				}
			}
			return walk(u.Elem())
		case *types.Slice:
			return walk(u.Elem())
		case *types.Array:
			return walk(u.Elem())
		case *types.Map:
			return walk(u.Key()) || walk(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if f := u.Field(i); (f.Exported() || promotes(f)) && walk(f.Type()) {
					return true
				}
			}
		case *types.Signature:
			return walkSig(u)
		}
		return false
	}
	// The wrappers of the package's functions and methods return an
	// http.Handler as a fetch handler (exportOut), not as a handle.
	exported := func(sig *types.Signature) bool {
		for i := 0; i < sig.Params().Len(); i++ {
			if walk(sig.Params().At(i).Type()) {
				return true
			}
		}
		for i := 0; i < sig.Results().Len(); i++ {
			if t := sig.Results().At(i).Type(); !isHTTPHandler(t) && walk(t) {
				return true
			}
		}
		return false
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		switch obj := scope.Lookup(name).(type) {
		case *types.Func:
			if obj.Exported() && obj.Signature().TypeParams().Len() == 0 && exported(obj.Signature()) {
				return true
			}
		case *types.TypeName:
			if n, ok := obj.Type().(*types.Named); ok && obj.Exported() && n.TypeParams().Len() == 0 {
				for _, fn := range handleMethods(n) {
					if exported(fn.Signature()) {
						return true
					}
				}
			}
		}
	}
	return false
}

// handleMethods are the methods JavaScript calls on a handle of type *n:
// the exported, non-generic methods declared on n or *n that no field of
// the same name hides.
func handleMethods(n *types.Named) []*types.Func {
	st, _ := n.Underlying().(*types.Struct)
	ms := types.NewMethodSet(types.NewPointer(n))
	var fns []*types.Func
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		fn := sel.Obj().(*types.Func)
		if !fn.Exported() || len(sel.Index()) != 1 || fn.Signature().TypeParams().Len() > 0 || st != nil && isFieldName(st, fn.Name()) {
			continue
		}
		fns = append(fns, fn)
	}
	return fns
}

// emitImportedHandleMethods gives the struct classes of importedHandles
// their exported methods, through export wrappers as the entry package's
// own methods have.
func (pe *pkgEmitter) emitImportedHandleMethods() {
	for _, n := range pe.importedHandles() {
		class := pe.structClass(n)
		for _, fn := range handleMethods(n) {
			name := pe.methodFuncName(fn)
			pe.exportWrapper(fn, name, "", pe.prog.IsAsync(fn))
			sig := fn.Signature()
			var params, args []string
			for j := 0; j < sig.Params().Len(); j++ {
				a := fmt.Sprintf("a%d", j)
				if sig.Variadic() && j == sig.Params().Len()-1 {
					params = append(params, "..."+a+": any[]")
					args = append(args, "..."+a)
					continue
				}
				params = append(params, a+": any")
				args = append(args, a)
			}
			pe.jsMethods = append(pe.jsMethods, fmt.Sprintf("(%s.prototype as any)[%s] = function (%s) { return %s(%s); };", class, jsString(fn.Name()),
				strings.Join(append([]string{"this: any"}, params...), ", "), pe.wrapperName(name), strings.Join(append([]string{"this"}, args...), ", ")))
		}
	}
}

// emitJSMethods gives the classes of handles their JS methods in $jsm. A
// handle reaches JavaScript only through the conversions of the JS calling
// ABI that take a type descriptor, so only those reference $jsm: a bundle
// whose exports convert no handles leaves the methods, their wrappers and
// the rest of $jsabi out.
func (pe *pkgEmitter) emitJSMethods() {
	if !pe.usesJSABI {
		return
	}
	if len(pe.jsMethods) == 0 {
		pe.funcs.ln("const $jsm = undefined;")
		return
	}
	pe.funcs.ln("const $jsm = /* @__PURE__ */ (() => {")
	for _, m := range pe.jsMethods {
		pe.funcs.ln("  %s", m)
	}
	pe.funcs.ln("})();")
}

// exportWrapper emits and exports, as exported, the function JavaScript
// calls for the entry package's function or method fn, lowered as name.
// Go code dereferences pointers without an explicit nil check (nilChecked),
// so a nil pointer is a JS TypeError until something converts it; the
// wrapper does, so that a panic reaches JS as a GoPanic.
func (pe *pkgEmitter) exportWrapper(fn *types.Func, name, exported string, async bool) {
	sig := fn.Signature()
	var vars []*types.Var
	if sig.Recv() != nil {
		vars = append(vars, sig.Recv())
	}
	for i := 0; i < sig.Params().Len(); i++ {
		vars = append(vars, sig.Params().At(i))
	}
	var ps, args []string
	for i, v := range vars {
		p := fmt.Sprintf("a%d", i)
		t := v.Type()
		if sig.Variadic() && i == len(vars)-1 {
			elem := t.(*types.Slice).Elem()
			ps = append(ps, "..."+p+": "+pe.jsTS(elem, true, nil)+"[]")
			args = append(args, "$jsabi.argToGo("+pe.typeDesc(t, tpScope{})+", "+p+", $jsm)")
			pe.usesJSABI = true
			continue
		}
		ts := pe.jsTS(t, true, nil)
		if i == 0 && sig.Recv() != nil && isHandleType(types.NewPointer(t)) {
			// A value method of a handle's type is called on the handle.
			ts = pe.structClass(t) + " | " + ts
		}
		ps = append(ps, p+": "+ts)
		args = append(args, pe.exportIn(t, p))
	}
	results := sig.Results()
	if async && results.Len() == 1 && isHTTPHandler(results.At(0).Type()) {
		// fetchHandler takes the Promise of the Handler: the fetch handler
		// is returned at once, and waits for it.
		async = false
	}
	call := fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
	kw := ""
	if async {
		kw, call = "async ", "await "+call
	}

	nres := results.Len()
	hasErr := nres > 0 && isErrorType(results.At(nres-1).Type())
	if hasErr {
		nres--
	}
	simple := !hasErr && results.Len() <= 1
	// result is the i-th result of the call: an async function's are in
	// the array it resolves to, the others' after the first in the result
	// registers, which the wrapper copies at once ($r1, ...), before any
	// conversion runs Go code (an error's Error method).
	result := func(i int) string {
		switch {
		case results.Len() <= 1:
			return "$r"
		case async:
			return fmt.Sprintf("$r[%d]", i)
		case i == 0:
			return "$r"
		}
		return fmt.Sprintf("$r%d", i)
	}
	var conv []string
	for i := 0; i < nres; i++ {
		x := call
		if !simple {
			x = result(i)
		}
		conv = append(conv, pe.exportOut(results.At(i).Type(), x))
	}
	ret := pe.exportResultTS(sig, async)

	w := pe.funcs
	js := pe.wrapperName(name)
	w.ln("%sfunction %s(%s): %s {", kw, js, strings.Join(ps, ", "), ret)
	w.indent++
	if simple {
		// One expression: V8 inlines name into the wrapper.
		w.ln("try {")
		if len(conv) == 0 {
			w.ln("  %s;", call)
		} else {
			w.ln("  return %s;", conv[0])
		}
		w.ln("} catch (e) {")
		w.ln("  throw $rt.toPanic(e);")
		w.ln("}")
	} else {
		regs := ""
		var reads []string
		if !async {
			for i := 1; i < results.Len(); i++ {
				regs += fmt.Sprintf(", $r%d", i)
				reads = append(reads, fmt.Sprintf(" $r%d = $rt.$R.r%d;", i, i))
			}
		}
		w.ln("let $r%s;", regs)
		w.ln("try {")
		w.ln("  $r = %s;%s", call, strings.Join(reads, ""))
		w.ln("} catch (e) {")
		w.ln("  throw $rt.toPanic(e);")
		w.ln("}")
		if hasErr {
			e := result(results.Len() - 1)
			w.ln("if (%s !== null) throw $jsabi.goError(%s);", e, e)
			pe.usesJSABI = true
		}
		switch len(conv) {
		case 0:
		case 1:
			w.ln("return %s;", conv[0])
		default:
			w.ln("return [%s];", strings.Join(conv, ", "))
		}
	}
	w.indent--
	w.ln("}")
	if exported != "" {
		pe.export(js, exported)
		if sig.Recv() == nil {
			pe.dtsFuncs = append(pe.dtsFuncs, "export function "+exported+pe.dtsSignature(fn, async)+";")
		}
	}
}

// dtsSignature is the TypeScript signature of the export wrapper of fn in
// the module's declaration file (moduleDTS), with the Go parameter names.
func (pe *pkgEmitter) dtsSignature(fn *types.Func, async bool) string {
	pe.dts = true
	defer func() { pe.dts = false }()
	sig := fn.Signature()
	var ps []string
	for i := 0; i < sig.Params().Len(); i++ {
		v := sig.Params().At(i)
		name := fmt.Sprintf("a%d", i)
		if n := v.Name(); n != "" && n != "_" {
			name = jsName(n)
		}
		if sig.Variadic() && i == sig.Params().Len()-1 {
			ps = append(ps, "..."+name+": "+pe.jsTS(v.Type().(*types.Slice).Elem(), true, nil)+"[]")
			continue
		}
		ps = append(ps, name+": "+pe.jsTS(v.Type(), true, nil))
	}
	return "(" + strings.Join(ps, ", ") + "): " + pe.exportResultTS(sig, async)
}

// moduleDTS is the declaration file of an entry package's module: what
// JavaScript calls, the exported functions and the classes of the
// package's handles with their methods, typed as the JS calling ABI
// converts. It names no other module, so that gosfc can declare a go:
// import with it.
func (pe *pkgEmitter) moduleDTS() string {
	if !pe.isEntry || pe.std || pe.dep {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by goesm from Go package %s. DO NOT EDIT.\n", pe.pkg.PkgPath)
	fmt.Fprintf(&b, "// The JavaScript calling ABI of its module (docs/js-exports.md).\n")
	for _, n := range handleTypes(pe.pkg.Types) {
		if n.Obj().Pkg() != pe.pkg.Types || n.TypeArgs().Len() > 0 {
			continue
		}
		fmt.Fprintf(&b, "export class %s {\n  private constructor();\n", pe.structClass(n))
		for _, fn := range handleMethods(n) {
			fmt.Fprintf(&b, "  %s%s;\n", jsPropName(fn.Name()), pe.dtsSignature(fn, pe.prog.IsAsync(fn)))
		}
		b.WriteString("}\n")
	}
	for _, f := range pe.dtsFuncs {
		b.WriteString(f + "\n")
	}
	return b.String()
}

// exportResultTS is the TypeScript result type of the export wrapper of a
// function of signature sig.
func (pe *pkgEmitter) exportResultTS(sig *types.Signature, async bool) string {
	return pe.exportResultTSSeen(sig, async, nil)
}

// handleMethodsTS lists the TypeScript signatures of the methods of a handle
// of type *n, from another package (see jsTS).
func (pe *pkgEmitter) handleMethodsTS(n *types.Named, seen map[types.Type]bool) string {
	sub := map[types.Type]bool{n: true}
	for k := range seen {
		sub[k] = true
	}
	var ms []string
	for _, fn := range handleMethods(n) {
		sig := fn.Signature()
		var ps []string
		for j := 0; j < sig.Params().Len(); j++ {
			pt := sig.Params().At(j).Type()
			if sig.Variadic() && j == sig.Params().Len()-1 {
				ps = append(ps, fmt.Sprintf("...a%d: %s[]", j, pe.jsTS(pt.(*types.Slice).Elem(), true, sub)))
				continue
			}
			ps = append(ps, fmt.Sprintf("a%d: %s", j, pe.jsTS(pt, true, sub)))
		}
		ms = append(ms, fmt.Sprintf("%s(%s): %s", jsPropName(fn.Name()), strings.Join(ps, ", "), pe.exportResultTSSeen(sig, pe.prog.IsAsync(fn), sub)))
	}
	return strings.Join(ms, "; ")
}

func (pe *pkgEmitter) exportResultTSSeen(sig *types.Signature, async bool, seen map[types.Type]bool) string {
	var rts []string
	for i := 0; i < sig.Results().Len(); i++ {
		t := sig.Results().At(i).Type()
		if i == sig.Results().Len()-1 && isErrorType(t) {
			break
		}
		rts = append(rts, pe.jsTS(t, false, seen))
	}
	ret := "void"
	switch len(rts) {
	case 0:
	case 1:
		ret = rts[0]
	default:
		ret = "[" + strings.Join(rts, ", ") + "]"
	}
	if async && !(sig.Results().Len() == 1 && isHTTPHandler(sig.Results().At(0).Type())) {
		ret = "Promise<" + ret + ">"
	}
	return ret
}

func isErrorType(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

// exportIn converts the JavaScript argument x to the Go type t.
func (pe *pkgEmitter) exportIn(t types.Type, x string) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch b.Kind() {
		case types.Bool:
			return "!!(" + x + ")"
		case types.String:
			pe.usesJSABI = true
			return "$jsabi.jsString(" + x + ")"
		case types.Int, types.Uint, types.Uintptr, types.Float64:
			// As they are: a call with numbers costs what a JS call does.
			// TypeScript types them number; a fraction for an int is the
			// caller's to avoid.
			return x
		case types.Int32:
			return "((" + x + ") | 0)"
		case types.Int64:
			pe.usesJSABI = true
			return "$jsabi.jsInt64(" + x + ")"
		case types.Uint64:
			pe.usesJSABI = true
			return "$jsabi.jsUint64(" + x + ")"
		}
	}
	pe.usesJSABI = true
	if sliceElemKind(t) == types.String {
		return "$jsabi.stringsToGo(" + x + ")"
	}
	return "$jsabi.argToGo(" + pe.typeDesc(t, tpScope{}) + ", " + x + ", $jsm)"
}

// sliceElemKind returns the kind of the elements of t when t is a slice of
// a basic type, and types.Invalid otherwise. Export wrappers convert the
// common slices with functions of their own, so that a bundle that needs no
// more of the JS calling ABI leaves the rest out.
func sliceElemKind(t types.Type) types.BasicKind {
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return types.Invalid
	}
	b, ok := s.Elem().Underlying().(*types.Basic)
	if !ok {
		return types.Invalid
	}
	return b.Kind()
}

// exportOut converts the Go result x of type t to JavaScript; x is used
// once.
func (pe *pkgEmitter) exportOut(t types.Type, x string) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		if b.Info()&types.IsString != 0 {
			return "$rt.toJSString(" + x + ")"
		}
		return x
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && isJSValue(t) && n.Obj().Name() == "Value" {
		return "$rt.fromRef(" + x + ".ref)"
	}
	if isHTTPHandler(t) {
		return "$rt.fetchHandler(" + x + ")"
	}
	pe.usesJSABI = true
	switch sliceElemKind(t) {
	case types.String:
		return "$jsabi.stringsToJS(" + x + ")"
	case types.Bool, types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint16, types.Uint32, types.Uint64, types.Uintptr, types.Float32, types.Float64:
		return "$jsabi.plainSliceToJS(" + x + ")"
	}
	return "$jsabi.resultToJS(" + pe.typeDesc(t, tpScope{}) + ", " + x + ", $jsm)"
}

// isHTTPHandler reports whether t is net/http's Handler, which JavaScript
// receives as a fetch handler (runtime/src/http.ts): a function from a
// Request to the Promise of a Response, for Cloudflare Workers, Deno.serve,
// Bun.serve and service workers.
func isHTTPHandler(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "net/http" && n.Obj().Name() == "Handler"
}

// isHandleType reports whether values of type t cross to JavaScript as Go
// values (see isHandle in runtime/src/jsabi.ts): pointers to struct types
// with methods, non-empty interfaces other than error, channels, complex
// numbers and unsafe pointers. A pointer to any other type is a handle in
// arguments only, and in results as its element.
func isHandleType(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if _, ok := u.Elem().Underlying().(*types.Struct); ok {
			return types.NewMethodSet(t).Len() > 0
		}
	case *types.Interface:
		return !u.Empty() && !isErrorType(t)
	case *types.Chan:
		return true
	case *types.Basic:
		return u.Info()&types.IsComplex != 0 || u.Kind() == types.UnsafePointer
	}
	return false
}

// jsTS is the TypeScript type JavaScript sees for a Go value of type t in
// an exported function's parameter (in) or result. seen holds the named
// types being rendered: a type defined in terms of itself is any at the
// recursion.
func (pe *pkgEmitter) jsTS(t types.Type, in bool, seen map[types.Type]bool) string {
	t = types.Unalias(t)
	if isJSValue(t) {
		return "any"
	}
	if !in && isHTTPHandler(t) {
		return "(request: Request, ...rest: any[]) => Promise<Response>"
	}
	if _, ok := t.(*types.Named); ok {
		if seen[t] {
			return "any"
		}
		m := map[types.Type]bool{t: true}
		for k := range seen {
			m[k] = true
		}
		seen = m
	}
	if isHandleType(t) {
		if p, ok := t.Underlying().(*types.Pointer); ok {
			if n, ok := types.Unalias(p.Elem()).(*types.Named); ok && n.TypeArgs().Len() == 0 {
				if pe.dts && n.Obj().Pkg() != pe.pkg.Types {
					// A declaration file imports no other module: the
					// handle is an object with its methods.
					if seen[n] {
						return "any"
					}
					if ms := pe.handleMethodsTS(n, seen); ms != "" {
						return "{ " + ms + " } | null"
					}
					return "object | null"
				}
				c := pe.structClass(n)
				if n.Obj().Pkg() != pe.pkg.Types && !seen[n] {
					// The class's TypeScript type has no methods: the
					// entry package adds them (emitImportedHandleMethods).
					if ms := pe.handleMethodsTS(n, seen); ms != "" {
						c = "(" + c + " & { " + ms + " })"
					}
				}
				return c + " | null"
			}
			return "any"
		}
		if pe.dts {
			return "unknown"
		}
		return pe.tsType(t, tpScope{})
	}
	if isErrorType(t) {
		return "Error | null"
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "boolean"
		case u.Info()&types.IsString != 0:
			return "string"
		case isBigKind(u):
			if in {
				return "bigint | number"
			}
			return "bigint"
		case u.Info()&types.IsNumeric != 0:
			return "number"
		}
	case *types.Slice:
		// A nil slice or map is an empty one in JavaScript; JavaScript may
		// pass null for one.
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			if in {
				return "Uint8Array | ArrayLike<number> | null"
			}
			return "Uint8Array"
		}
		return "Array<" + pe.jsTS(u.Elem(), in, seen) + ">" + orNull(in)
	case *types.Array:
		return "Array<" + pe.jsTS(u.Elem(), in, seen) + ">"
	case *types.Map:
		if b, ok := u.Key().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
			return "Record<string, " + pe.jsTS(u.Elem(), in, seen) + ">" + orNull(in)
		}
		return "Map<" + pe.jsTS(u.Key(), in, seen) + ", " + pe.jsTS(u.Elem(), in, seen) + ">" + orNull(in)
	case *types.Struct:
		return pe.jsStructTS(u, in, seen)
	case *types.Pointer:
		elem := pe.jsTS(u.Elem(), in, seen)
		if elem == "any" {
			return "any"
		}
		return elem + " | null"
	case *types.Signature:
		// A function goes the other way: its parameters come from the side
		// that receives it.
		var ps []string
		for i := 0; i < u.Params().Len(); i++ {
			pt := u.Params().At(i).Type()
			if u.Variadic() && i == u.Params().Len()-1 {
				ps = append(ps, fmt.Sprintf("...a%d: %s[]", i, pe.jsTS(pt.(*types.Slice).Elem(), !in, seen)))
				continue
			}
			ps = append(ps, fmt.Sprintf("a%d: %s", i, pe.jsTS(pt, !in, seen)))
		}
		var rs []string
		for i := 0; i < u.Results().Len(); i++ {
			rt := u.Results().At(i).Type()
			if i == u.Results().Len()-1 && isErrorType(rt) {
				break
			}
			rs = append(rs, pe.jsTS(rt, in, seen))
		}
		ret := "void"
		switch len(rs) {
		case 0:
		case 1:
			ret = rs[0]
		default:
			ret = "[" + strings.Join(rs, ", ") + "]"
		}
		if !in {
			// A Go function that blocks returns a Promise.
			ret += " | Promise<" + ret + ">"
		}
		return "((" + strings.Join(ps, ", ") + ") => " + ret + ") | null"
	}
	return "any"
}

// jsStructTS is the object type of a struct value: its exported fields,
// named as encoding/json names them; in an argument, each may be left out.
func (pe *pkgEmitter) jsStructTS(st *types.Struct, in bool, seen map[types.Type]bool) string {
	opt := ""
	if in {
		opt = "?"
	}
	var props []string
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		name := jsFieldName(st.Tag(i))
		if name == "-" {
			continue
		}
		if promotes(f) && name == "" {
			ft := f.Type()
			if p, ok := ft.Underlying().(*types.Pointer); ok {
				ft = types.Unalias(p.Elem())
			}
			// A struct embedding itself (type Node struct { *Node }) adds
			// no fields at the recursion, as in encoding/json.
			sub := seen
			if _, ok := ft.(*types.Named); ok {
				if seen[ft] {
					continue
				}
				sub = map[types.Type]bool{ft: true}
				for k := range seen {
					sub[k] = true
				}
			}
			if inner := pe.jsStructTS(ft.Underlying().(*types.Struct), in, sub); inner != "{}" {
				props = append(props, strings.TrimSuffix(strings.TrimPrefix(inner, "{ "), " }"))
			}
			continue
		}
		if !f.Exported() {
			continue
		}
		if name == "" {
			name = f.Name()
		}
		key := name
		if !isJSIdent(key) {
			key = jsStringLit(name)
		}
		props = append(props, key+opt+": "+pe.jsTS(f.Type(), in, seen))
	}
	if len(props) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(props, "; ") + " }"
}

// orNull is " | null" for a parameter (in), which may be null.
func orNull(in bool) string {
	if in {
		return " | null"
	}
	return ""
}

// jsFieldName is the name of a struct field with the given tag in
// JavaScript objects: the name in its js tag, or else in its json tag, as
// encoding/json names fields; "" for the Go name, "-" for a field that does
// not cross.
func jsFieldName(tag string) string {
	st := reflect.StructTag(tag)
	v, ok := st.Lookup("js")
	if !ok {
		v = st.Get("json")
	}
	name, _, _ := strings.Cut(v, ",")
	return name
}

// isJSIdent reports whether s can be written unquoted as a property name.
func isJSIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c == '_' || c == '$' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || i > 0 && '0' <= c && c <= '9' {
			continue
		}
		return false
	}
	return true
}
