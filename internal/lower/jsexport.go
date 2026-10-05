package lower

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"
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
	js := pe.fresh(name + "$js")
	pe.wrappers[name] = js
	return js
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
			args = append(args, "$jsabi.argToGo("+pe.typeDesc(t, tpScope{})+", "+p+")")
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
	var conv []string
	for i := 0; i < nres; i++ {
		x := "$r"
		switch {
		case simple:
			x = call
		case results.Len() > 1:
			x = fmt.Sprintf("$r[%d]", i)
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
		w.ln("let $r;")
		w.ln("try {")
		w.ln("  $r = %s;", call)
		w.ln("} catch (e) {")
		w.ln("  throw $rt.toPanic(e);")
		w.ln("}")
		if hasErr {
			e := "$r"
			if results.Len() > 1 {
				e = fmt.Sprintf("$r[%d]", results.Len()-1)
			}
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
	pe.export(js, exported)
}

// exportResultTS is the TypeScript result type of the export wrapper of a
// function of signature sig.
func (pe *pkgEmitter) exportResultTS(sig *types.Signature, async bool) string {
	var rts []string
	for i := 0; i < sig.Results().Len(); i++ {
		t := sig.Results().At(i).Type()
		if i == sig.Results().Len()-1 && isErrorType(t) {
			break
		}
		rts = append(rts, pe.jsTS(t, false, nil))
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
	return "$jsabi.argToGo(" + pe.typeDesc(t, tpScope{}) + ", " + x + ")"
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
	return "$jsabi.resultToJS(" + pe.typeDesc(t, tpScope{}) + ", " + x + ")"
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
				return pe.structClass(n) + " | null"
			}
			return "any"
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
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			if in {
				return "Uint8Array | ArrayLike<number> | null"
			}
			return "Uint8Array | null"
		}
		return "Array<" + pe.jsTS(u.Elem(), in, seen) + "> | null"
	case *types.Array:
		return "Array<" + pe.jsTS(u.Elem(), in, seen) + ">"
	case *types.Map:
		if b, ok := u.Key().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
			return "Record<string, " + pe.jsTS(u.Elem(), in, seen) + "> | null"
		}
		return "Map<" + pe.jsTS(u.Key(), in, seen) + ", " + pe.jsTS(u.Elem(), in, seen) + "> | null"
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
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if promotes(f) && name == "" {
			ft := f.Type()
			if p, ok := ft.Underlying().(*types.Pointer); ok {
				ft = p.Elem()
			}
			if inner := pe.jsStructTS(ft.Underlying().(*types.Struct), in, seen); inner != "{}" {
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
