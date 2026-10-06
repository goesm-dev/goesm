package lower

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// //goesm:import: Go calling JavaScript and TypeScript.
//
// A function declared without a body, or a package variable declared
// without a value, imports an export of an ES module:
//
//	//goesm:import "./format.ts" formatPrice
//	func formatPrice(yen int) string
//
//	//goesm:import "chart.js" Chart
//	var Chart js.Value
//
//	//goesm:import "./api.ts" fetchUser await
//	func fetchUser(id string) (User, error)
//
// The directive names the module as an import specifier would (a relative
// path is relative to the Go file, anything else is left to the bundler),
// then the export: a name, "default" (also when it is left out) or "*" for
// the module namespace object. A trailing "await" marks a function that
// returns a Promise: Go calls it as a function that blocks until the
// Promise settles.
//
// Arguments and results are converted at the boundary according to the
// declared Go types (runtime/src/jsabi.ts); the declaration is the only
// description of the JS function goesm has. A final error result receives
// an exception or a rejection as an error; without one, they panic.

// jsImport is a parsed //goesm:import directive.
type jsImport struct {
	spec, name string
	await      bool
	pos        token.Pos // of the directive
}

// jsImportDirective returns the //goesm:import directive of doc, if any.
func jsImportDirective(doc *ast.CommentGroup) (*jsImport, token.Pos, error) {
	if doc == nil {
		return nil, token.NoPos, nil
	}
	for _, c := range doc.List {
		rest, ok := strings.CutPrefix(c.Text, "//goesm:import")
		if !ok || rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		d, err := parseJSImport(strings.TrimSpace(rest))
		if d != nil {
			d.pos = c.Pos()
		}
		return d, c.Pos(), err
	}
	return nil, token.NoPos, nil
}

func parseJSImport(s string) (*jsImport, error) {
	q, err := strconv.QuotedPrefix(s)
	if err != nil {
		return nil, fmt.Errorf(`//goesm:import needs a quoted module specifier: //goesm:import "./module.ts" name`)
	}
	spec, _ := strconv.Unquote(q)
	if spec == "" {
		return nil, fmt.Errorf("//goesm:import: empty module specifier")
	}
	d := &jsImport{spec: spec, name: "default"}
	fields := strings.Fields(s[len(q):])
	if n := len(fields); n > 0 && fields[n-1] == "await" {
		d.await = true
		fields = fields[:n-1]
	}
	switch len(fields) {
	case 0:
	case 1:
		d.name = fields[0]
	default:
		return nil, fmt.Errorf("//goesm:import takes a module, an export name and await, got %q", s)
	}
	return d, nil
}

// funcJSImport returns the directive of a function declaration that imports
// a JS function: one without a body.
func funcJSImport(fd *ast.FuncDecl) *jsImport {
	if fd.Body != nil || fd.Recv != nil {
		return nil
	}
	d, _, err := jsImportDirective(fd.Doc)
	if err != nil {
		return nil
	}
	return d
}

// varJSImports returns the //goesm:import directives of the package
// variables declared in files.
func (pe *pkgEmitter) varJSImports(files []*ast.File) map[*types.Var]*jsImport {
	m := map[*types.Var]*jsImport{}
	for _, f := range files {
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				doc := vs.Doc
				if doc == nil && len(g.Specs) == 1 {
					doc = g.Doc
				}
				d, pos, err := jsImportDirective(doc)
				if err != nil {
					pe.errorf(pos, "%v", err)
					continue
				}
				if d == nil {
					continue
				}
				if len(vs.Names) != 1 || len(vs.Values) != 0 {
					pe.errorf(pos, "//goesm:import must precede a declaration of one variable without a value")
					continue
				}
				if d.await {
					pe.errorf(pos, "//goesm:import: await applies only to functions")
					continue
				}
				v, ok := pe.info.Defs[vs.Names[0]].(*types.Var)
				if !ok {
					continue
				}
				if why := jsABIType(v.Type(), false, map[types.Type]bool{}); why != "" {
					pe.errorf(pos, "variable %s cannot be imported from JavaScript: %s", v.Name(), why)
					continue
				}
				m[v] = d
			}
		}
	}
	return m
}

// jsStringLit is s as a JS string literal holding the same characters (not
// goesm's byte strings): module specifiers and export names.
func jsStringLit(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// JSImportSpecifier is the specifier a module written to file imports the
// file target with: generated modules import the JS and TS files of
// //goesm:import directives by absolute path, recorded in Module.Files, and
// internal/build makes them relative to where it writes the module.
func JSImportSpecifier(file, target string) (old, new string) {
	file, err := filepath.Abs(file)
	if err != nil {
		return "", ""
	}
	rel, err := filepath.Rel(filepath.Dir(file), target)
	if err != nil {
		return "", ""
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "../") {
		rel = "./" + rel
	}
	// Only the module specifier of an import declaration: a string literal
	// in the code that happens to equal the path stays as it is.
	return " from " + jsStringLit(target) + ";\n", " from " + jsStringLit(rel) + ";\n"
}

// jsImportBinding returns the module-local name of the export d imports,
// adding the import to the module.
func (pe *pkgEmitter) jsImportBinding(d *jsImport) string {
	pos := d.pos
	spec := d.spec
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		// Relative to the Go file, or to the file a //line directive names
		// (a Vue component's Go block, for gosfc).
		dir := filepath.Dir(pe.prog.Fset.Position(pos).Filename)
		abs := filepath.Join(dir, filepath.FromSlash(spec))
		if _, err := os.Stat(abs); err != nil {
			pe.errorf(pos, "//goesm:import: %s does not exist", spec)
		}
		spec = abs
		if !pe.jsFileSet[abs] {
			pe.jsFileSet[abs] = true
			pe.jsFiles = append(pe.jsFiles, abs)
		}
	}
	key := spec + "\x00" + d.name
	if name, ok := pe.jsImportNames[key]; ok {
		return name
	}
	local := pe.fresh("js")
	pe.reserved[local] = true
	pe.jsImportNames[key] = local
	m := pe.jsModules[spec]
	if m == nil {
		m = &jsModule{spec: spec}
		pe.jsModules[spec] = m
		pe.jsModuleOrder = append(pe.jsModuleOrder, m)
	}
	switch {
	case d.name == "*":
		m.namespace = local
	case d.name == "default":
		m.def = local
	case token.IsIdentifier(d.name):
		m.named = append(m.named, d.name+" as "+local)
	default:
		m.named = append(m.named, jsStringLit(d.name)+" as "+local)
	}
	return local
}

// jsModule is an ES module imported by //goesm:import directives.
type jsModule struct {
	spec           string
	def, namespace string   // local names of the default export and the namespace
	named          []string // "export as local"
}

// jsImportDecls returns the import declarations of the module's
// //goesm:import directives.
func (pe *pkgEmitter) jsImportDecls() []string {
	var out []string
	for _, m := range pe.jsModuleOrder {
		spec := jsStringLit(m.spec)
		var clauses []string
		if m.def != "" {
			clauses = append(clauses, m.def)
		}
		if len(m.named) > 0 {
			clauses = append(clauses, "{ "+strings.Join(m.named, ", ")+" }")
		}
		if len(clauses) > 0 {
			out = append(out, fmt.Sprintf("import %s from %s;", strings.Join(clauses, ", "), spec))
		}
		if m.namespace != "" {
			out = append(out, fmt.Sprintf("import * as %s from %s;", m.namespace, spec))
		}
	}
	return out
}

// promotes reports whether the embedded field f promotes its fields into
// the JavaScript object, as encoding/json flattens an embedded struct or
// pointer to struct, exported or not.
func promotes(f *types.Var) bool {
	if !f.Embedded() {
		return false
	}
	t := f.Type()
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	_, ok := t.Underlying().(*types.Struct)
	return ok
}

// isJSValue reports whether t is syscall/js's Value or Func.
func isJSValue(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != "syscall/js" {
		return false
	}
	return n.Obj().Name() == "Value" || n.Obj().Name() == "Func"
}

// jsABIType says why a value of type t cannot cross the JS boundary, or
// returns "". toJS is the direction: Go to JavaScript.
func jsABIType(t types.Type, toJS bool, seen map[types.Type]bool) string {
	if isJSValue(t) {
		return ""
	}
	if seen[t] {
		return ""
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0 && u.Info()&types.IsComplex == 0 {
			return ""
		}
	case *types.Slice:
		return jsABIType(u.Elem(), toJS, seen)
	case *types.Array:
		return jsABIType(u.Elem(), toJS, seen)
	case *types.Map:
		if b, ok := u.Key().Underlying().(*types.Basic); !ok || b.Info()&types.IsString == 0 {
			return fmt.Sprintf("the keys of %s are not strings", t)
		}
		return jsABIType(u.Elem(), toJS, seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if f := u.Field(i); f.Exported() || promotes(f) {
				if why := jsABIType(f.Type(), toJS, seen); why != "" {
					return why
				}
			}
		}
		return ""
	case *types.Pointer:
		if _, ok := u.Elem().Underlying().(*types.Struct); ok {
			return jsABIType(u.Elem(), toJS, seen)
		}
	case *types.Interface:
		if toJS || u.Empty() {
			return ""
		}
	case *types.Signature:
		if !toJS {
			return "a JavaScript function is received as a js.Value"
		}
		for i := 0; i < u.Params().Len(); i++ {
			if why := jsABIType(u.Params().At(i).Type(), false, seen); why != "" {
				return why
			}
		}
		for i := 0; i < u.Results().Len(); i++ {
			if why := jsABIType(u.Results().At(i).Type(), true, seen); why != "" {
				return why
			}
		}
		return ""
	}
	return fmt.Sprintf("%s has no JavaScript counterpart", t)
}

// toJSExpr converts the Go value x of type t to JavaScript.
func (pe *pkgEmitter) toJSExpr(t types.Type, x string) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		if b.Info()&types.IsString != 0 {
			return "$rt.toJSString(" + x + ")"
		}
		return x
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && isJSValue(t) && n.Obj().Name() == "Value" {
		return "$rt.fromRef(" + x + ".ref)"
	}
	if lit := pe.structToJS(t, x); lit != "" {
		return lit
	}
	return "$jsabi.goToJS(" + pe.typeDesc(t, tpScope{}) + ", " + x + ")"
}

// structToJS converts the struct (or pointer to struct) x, an identifier,
// to an object literal, if every field that crosses is a string, a boolean
// or a number; "" otherwise. It is goToJS in the runtime unrolled for t.
func (pe *pkgEmitter) structToJS(t types.Type, x string) string {
	ptr := false
	if p, ok := t.Underlying().(*types.Pointer); ok {
		ptr, t = true, p.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok || isJSValue(t) {
		return ""
	}
	var props []string
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			return "" // promoted fields are goToJS's
		}
		if !f.Exported() {
			continue
		}
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		b, ok := f.Type().Underlying().(*types.Basic)
		if !ok || b.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) == 0 || b.Info()&types.IsComplex != 0 {
			return ""
		}
		if name == "" {
			name = f.Name()
		}
		if name == "__proto__" {
			return "" // a literal would set the prototype
		}
		v := x + "." + fieldProp(st, i)
		if b.Info()&types.IsString != 0 {
			v = "$rt.toJSString(" + v + ")"
		}
		props = append(props, jsStringLit(name)+": "+v)
	}
	lit := "{ " + strings.Join(props, ", ") + " }"
	if ptr {
		return "(" + x + " === null ? null : " + lit + ")"
	}
	return lit
}

// fromJSExpr converts the JavaScript value x to the Go type t.
func (pe *pkgEmitter) fromJSExpr(t types.Type, x string) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch b.Kind() {
		case types.Bool:
			return "!!(" + x + ")"
		case types.String:
			return "$jsabi.jsString(" + x + ")"
		case types.Float64:
			return "+(" + x + ")"
		case types.Int32:
			return "((" + x + ") | 0)"
		case types.Int, types.Uint, types.Uintptr:
			return "$jsabi.jsInt(" + x + ")"
		}
	}
	return "$jsabi.jsToGo(" + pe.typeDesc(t, tpScope{}) + ", " + x + ")"
}

// jsErrorDesc is the descriptor of the error type JavaScript exceptions
// become in this package: js.Error if the package imports syscall/js,
// otherwise the runtime's own (see jsError in runtime/src/jsabi.ts).
func (pe *pkgEmitter) jsErrorDesc() string {
	for ip := range pe.direct {
		if ip.Path() == "syscall/js" {
			if tn, ok := ip.Scope().Lookup("Error").(*types.TypeName); ok {
				return pe.typeDesc(tn.Type(), tpScope{})
			}
		}
	}
	return "null"
}

// emitJSImportFunc emits the function fd that calls the JS export d.
func (pe *pkgEmitter) emitJSImportFunc(fd *ast.FuncDecl, fn *types.Func, d *jsImport, name string) {
	sig := fn.Signature()
	if sig.TypeParams().Len() > 0 {
		pe.errorf(d.pos, "a generic function cannot be imported from JavaScript")
		return
	}
	params, results := sig.Params(), sig.Results()
	nres := results.Len()
	hasErr := nres > 0 && types.Identical(results.At(nres-1).Type(), types.Universe.Lookup("error").Type())
	if hasErr {
		nres--
	}
	for i := 0; i < params.Len(); i++ {
		t := params.At(i).Type()
		if sig.Variadic() && i == params.Len()-1 {
			t = t.(*types.Slice).Elem()
		}
		if why := jsABIType(t, true, map[types.Type]bool{}); why != "" {
			pe.errorf(d.pos, "%s cannot be imported from JavaScript: parameter %d: %s", fn.Name(), i+1, why)
			return
		}
	}
	for i := 0; i < nres; i++ {
		if why := jsABIType(results.At(i).Type(), false, map[types.Type]bool{}); why != "" {
			pe.errorf(d.pos, "%s cannot be imported from JavaScript: result %d: %s", fn.Name(), i+1, why)
			return
		}
	}
	f := pe.jsImportBinding(d)
	var ps, args []string
	for i := 0; i < params.Len(); i++ {
		p := fmt.Sprintf("a%d", i)
		ps = append(ps, p+": any")
		if sig.Variadic() && i == params.Len()-1 {
			elem := params.At(i).Type().(*types.Slice).Elem()
			args = append(args, "...$jsabi.goArgsToJS("+pe.typeDesc(elem, tpScope{})+", "+p+")")
			continue
		}
		args = append(args, pe.toJSExpr(params.At(i).Type(), p))
	}
	call := f + "(" + strings.Join(args, ", ") + ")"
	kw, ret := "", "any"
	if d.await {
		kw, ret = "async ", "Promise<any>"
		call = "await " + call
	}
	var conv []string
	switch nres {
	case 0:
	case 1:
		conv = []string{pe.fromJSExpr(results.At(0).Type(), "$r")}
	default:
		for i := 0; i < nres; i++ {
			conv = append(conv, pe.fromJSExpr(results.At(i).Type(), fmt.Sprintf("$r[%d]", i)))
		}
	}
	errT := pe.jsErrorDesc()
	w := pe.funcs
	w.ln("%s%sfunction %s(%s): %s {", pe.tab.mark(fd.Pos()), kw, name, strings.Join(ps, ", "), ret)
	w.indent++
	w.ln("let $r;")
	w.ln("try {")
	w.ln("  $r = %s;", call)
	w.ln("} catch ($e) {")
	if hasErr {
		var zeros []string
		for i := 0; i < nres; i++ {
			zeros = append(zeros, pe.zeroOf(results.At(i).Type(), tpScope{}))
		}
		zeros = append(zeros, "$jsabi.jsError($e, "+errT+")")
		w.ln("  return %s;", multiResult(zeros, d.await))
	} else {
		w.ln("  $jsabi.jsPanic($e, %s);", errT)
	}
	w.ln("}")
	if !d.await && (nres > 1 || hasErr || nres == 1 && !isJSValue(results.At(0).Type())) {
		// A function that returns a Promise needs await; without it, the
		// Promise would convert to a meaningless value (or hide a
		// rejection) without notice. A js.Value result can hold a Promise
		// on purpose, and a function without results may be left running.
		msg := fmt.Sprintf("goesm: %s.%s imported from %q returned a Promise; mark its //goesm:import directive with await", pe.pkg.Types.Name(), fn.Name(), d.spec)
		w.ln("if (typeof $r === \"object\" && $r instanceof Promise) $jsabi.notAwaited($r, %s);", jsStringLit(msg))
	}
	if hasErr {
		conv = append(conv, "null")
	}
	if len(conv) > 0 {
		w.ln("return %s;", multiResult(conv, d.await))
	}
	w.indent--
	w.ln("}")
}

// emitJSImportVars initializes the package variables imported from
// JavaScript, before the package's other variables (the modules they come
// from are evaluated before this one).
func (pe *pkgEmitter) emitJSImportVars(files []*ast.File, fe *funcEmitter) {
	vars := pe.varJSImports(files)
	scope := pe.pkg.Types.Scope()
	for _, name := range scope.Names() {
		v, ok := scope.Lookup(name).(*types.Var)
		if !ok || vars[v] == nil {
			continue
		}
		local := pe.jsImportBinding(vars[v])
		pe.vars.ln("%s%s = %s;", pe.tab.mark(v.Pos()), fe.varRef(v), pe.fromJSExpr(v.Type(), local))
	}
}
