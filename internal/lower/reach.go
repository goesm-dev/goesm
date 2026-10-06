package lower

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Which interface methods the program calls. A method table entry keeps
// its method's function alive in a bundle, and with it everything that
// function calls, so a table holds the functions only of the methods that
// can be called through it: those implementing an interface method that
// reachable code calls (CalledMethod). Go's linker prunes method tables the
// same way.
//
// Reachable code starts from the roots: every function of the packages of
// the program's own module (neither standard library nor third-party, with
// the entry package always among them), package initialization, the
// functions provided to //go:linkname pulls, and the methods JavaScript
// calls on handles (jsexport.go). A reference to a function or a concrete
// method makes it reachable; a call, method value or method expression of
// an interface method or a type parameter's method (also one promoted
// through an embedded interface) marks that method called, which makes
// every method implementing it reachable. The runtime calls Error and
// String itself, but only on the values it prints or hands to JavaScript:
// a panic value (any type's methods if it has an interface type, as in
// panic(err), only its own type's otherwise, as syscall/js's
// panic(&ValueError{...})), and an error or other interface value crossing
// the JavaScript boundary (an export's or an import's, see
// jsBoundaryIfaces). A program that does neither, such as one that calls
// strconv.Atoi and only compares the error with nil, leaves out
// NumError's Error and the quoting and Unicode tables it needs. A program
// that enumerates methods by reflection may call any exported method.

// Reachable code also decides whether regexp/syntax needs the tables of
// package unicode's categories and scripts, which take a third of a
// bundle that parses Markdown: only to parse \p and \P (UnicodeClasses).

// patternFuncs are the functions that parse a regular expression given as
// their first argument.
var patternFuncs = map[string]bool{
	"regexp.Compile": true, "regexp.CompilePOSIX": true, "regexp.MustCompile": true, "regexp.MustCompilePOSIX": true,
	"regexp.Match": true, "regexp.MatchString": true, "regexp.MatchReader": true,
	"regexp/syntax.Parse": true,
}

// regexpJSFuncs are the functions that compile their first argument as
// regexp.Compile does: with a pattern known at compile time that
// translateRegexp translates, the program can match with a RegExp
// (regexpjs.go).
var regexpJSFuncs = map[string]bool{
	"regexp.Compile": true, "regexp.MustCompile": true, "regexp.Match": true, "regexp.MatchString": true,
}

// regexpGoFuncs need Go's engine: leftmost-longest matching, a RuneReader
// read as far as the match needs, or a pattern given at run time.
var regexpGoFuncs = map[string]bool{
	"regexp.CompilePOSIX": true, "regexp.MustCompilePOSIX": true, "regexp.MatchReader": true,
	"(*regexp.Regexp).Longest": true, "(*regexp.Regexp).MatchReader": true,
	"(*regexp.Regexp).FindReaderIndex": true, "(*regexp.Regexp).FindReaderSubmatchIndex": true,
	"(*regexp.Regexp).UnmarshalText": true,
}

// RegexpJS returns the jsPattern entries of the patterns the program
// compiles, if it matches every regular expression with a RegExp
// (regexpjs.go), or nil.
func (p *Program) RegexpJS() map[string]string {
	p.reachOnce.Do(p.reach)
	if p.regexpGo || len(p.regexpPatterns) == 0 {
		return nil
	}
	return p.regexpPatterns
}

// UnicodeClasses reports whether the program may parse a regular expression
// with a Unicode class (\p or \P): whether reachable code outside regexp
// passes a pattern that is not a constant to one of patternFuncs, or one
// with \p or \P, refers to one without calling it, or reaches
// (*regexp.Regexp).UnmarshalText. regexp/syntax's unicodeTable is not
// lowered otherwise.
func (p *Program) UnicodeClasses() bool {
	p.reachOnce.Do(p.reach)
	return p.unicodeClasses
}

// regexpLeftOut are the functions of package regexp that a program
// matching with RegExps never calls: Go's parser and engines, and the
// quoting of a pattern that failed to compile (leftOut in emit.go).
var regexpLeftOut = map[string]bool{"regexp.compileGo": true, "(*regexp.Regexp).findGo": true, "regexp.quote": true, "regexp.compileError": true}

// reach computes calledMethods, unicodeClasses, regexpPatterns and
// regexpGo. If the program can match with RegExps, it does so again
// without what that leaves out, whose panics, for one, would make the
// runtime call Error methods.
func (p *Program) reach() {
	p.reachFrom(nil)
	if !p.regexpGo && len(p.regexpPatterns) > 0 {
		p.reachFrom(regexpLeftOut)
	}
}

func (p *Program) reachFrom(leftOut map[string]bool) {
	p.calledMethods = map[string][]ifaceMethod{}
	p.printedMethods = map[*types.Func]bool{}
	p.unicodeClasses, p.regexpGo = false, false
	type body struct {
		node ast.Node
		info *types.Info
		pkg  string
	}
	decls := map[*types.Func]body{}
	byName := map[string][]*types.Func{} // concrete methods with bodies
	var queue []body
	for _, pkg := range p.Pkgs {
		root := !p.std[pkg] && !p.Deps[pkg] || pkg.PkgPath == p.Entry
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body == nil {
						continue
					}
					b := body{d.Body, pkg.TypesInfo, pkg.PkgPath}
					fn, _ := pkg.TypesInfo.Defs[d.Name].(*types.Func)
					if root || fn == nil || d.Recv == nil && d.Name.Name == "init" {
						queue = append(queue, b)
						continue
					}
					decls[fn] = b
					if d.Recv != nil {
						byName[fn.Name()] = append(byName[fn.Name()], fn)
					}
				case *ast.GenDecl:
					if d.Tok == token.VAR {
						queue = append(queue, body{d, pkg.TypesInfo, pkg.PkgPath})
					}
				}
			}
		}
	}

	seen := map[*types.Func]bool{}
	use := func(fn *types.Func) {
		fn = fn.Origin()
		if fn.FullName() == "(*regexp.Regexp).UnmarshalText" {
			p.unicodeClasses = true
		}
		if regexpGoFuncs[fn.FullName()] {
			p.regexpGo = true
		}
		if b, ok := decls[fn]; ok && !seen[fn] && !leftOut[fn.FullName()] {
			seen[fn] = true
			queue = append(queue, b)
		}
	}
	called := func(m *types.Func) {
		m = m.Origin()
		im := ifaceMethod{m, stripRecv(m.Signature()), hasTypeParam(m.Signature())}
		for _, c := range p.calledMethods[m.Name()] {
			if c.fn == m {
				return
			}
		}
		p.calledMethods[m.Name()] = append(p.calledMethods[m.Name()], im)
		for _, fn := range byName[m.Name()] {
			if matchMethod([]ifaceMethod{im}, fn) {
				use(fn)
			}
		}
	}

	runtimeCalls := false
	runtimeCall := func() {
		if runtimeCalls {
			return
		}
		runtimeCalls = true
		errorIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		called(errorIface.Method(0))
		for _, m := range p.ifaceMethods["String"] {
			if m.fn.Pkg() == nil { // the runtime's String() string (findDynMethods)
				called(m.fn)
			}
		}
	}
	if p.jsBoundaryIfaces() {
		runtimeCall()
	}
	for fn := range p.linkProvides {
		use(fn)
	}
	if p.allMethods {
		for _, fns := range byName {
			for _, fn := range fns {
				if fn.Exported() {
					use(fn)
				}
			}
		}
	}
	for _, pkg := range p.Pkgs {
		if pkg.PkgPath == p.Entry {
			for _, n := range handleTypes(pkg.Types) {
				for _, fn := range handleMethods(n) {
					use(fn)
				}
			}
		}
	}

	strs := &knownStrings{byTypes: p.byTypes}
	constPattern := map[*ast.Ident]bool{} // a patternFuncs callee with a pattern known at compile time, without \p
	knownPattern := map[*ast.Ident]string{}
	p.regexpPatterns = map[string]string{}
	for len(queue) > 0 {
		b := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		ast.Inspect(b.node, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && len(call.Args) == 1 && !runtimeCalls {
				// The runtime prints a panic value with its Error or
				// String method (formatPanicValue), as soon as it panics:
				// those of any type for a value of interface type, else
				// those of its type.
				if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok {
					if bi, ok := b.info.Uses[id].(*types.Builtin); ok && bi.Name() == "panic" {
						switch t := b.info.TypeOf(call.Args[0]); t.Underlying().(type) {
						case *types.Interface:
							if !recovered(b.info, b.node, call.Args[0]) {
								runtimeCall()
							}
						default:
							ms := types.NewMethodSet(t)
							for _, name := range []string{"Error", "String"} {
								if sel := ms.Lookup(nil, name); sel != nil {
									fn := sel.Obj().(*types.Func)
									if sig := fn.Signature(); sig.Params().Len() == 0 && sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), types.Typ[types.String]) {
										p.printedMethods[fn.Origin()] = true
										use(fn)
									}
								}
							}
						}
					}
				}
			}
			if call, ok := n.(*ast.CallExpr); ok && len(call.Args) > 0 {
				var id *ast.Ident
				switch f := ast.Unparen(call.Fun).(type) {
				case *ast.Ident:
					id = f
				case *ast.SelectorExpr:
					id = f.Sel
				}
				if s, ok := strs.known(b.info, call.Args[0]); id != nil && ok {
					constPattern[id] = !strings.Contains(s, `\p`) && !strings.Contains(s, `\P`)
					knownPattern[id] = s
				}
				return true
			}
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			fn, ok := b.info.Uses[id].(*types.Func)
			if !ok {
				return true
			}
			// regexp's own calls pass on its callers' patterns.
			if patternFuncs[fn.FullName()] && !constPattern[id] && b.pkg != "regexp" && b.pkg != "regexp/syntax" {
				p.unicodeClasses = true
			}
			if regexpJSFuncs[fn.FullName()] && b.pkg != "regexp" {
				s, ok := knownPattern[id]
				if ok {
					if _, done := p.regexpPatterns[s]; !done {
						p.regexpPatterns[s], _ = translateRegexp(s)
					}
					ok = p.regexpPatterns[s] != ""
				}
				if !ok {
					p.regexpGo = true
				}
			}
			if abstractMethod(fn) {
				called(fn)
				return true
			}
			use(fn)
			if pkg := fn.Pkg(); pkg != nil && pkg.Path() == "fmt" && (fn.Name() == "Sprintf" || fn.Name() == "Errorf") {
				// The lowering of these calls calls fmt's helpers
				// (sprintf.go).
				for _, name := range []string{"newError", "panicText"} {
					if h, ok := pkg.Scope().Lookup(name).(*types.Func); ok {
						use(h)
					}
				}
			}
			return true
		})
	}
}

// abstractMethod reports whether fn is a method of an interface, or of a
// type parameter's constraint.
func abstractMethod(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	t := types.Unalias(recv.Type())
	if _, ok := t.(*types.TypeParam); ok {
		return true
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// knownStrings evaluates string expressions known at compile time: string
// constants, concatenations of known strings, and the unexported
// package-level variables of string type initialized with a known string
// that their package never assigns again, such as the parts goldmark puts
// its HTML patterns together from.
type knownStrings struct {
	byTypes map[*types.Package]*packages.Package
	vars    map[*types.Var]*knownVar
	pkgs    map[*types.Package]bool // packages whose variables are in vars
}

type knownVar struct {
	init    ast.Expr
	info    *types.Info
	written bool
	state   int // 0 not evaluated, 1 evaluating, 2 done
	val     string
	ok      bool
}

func (k *knownStrings) known(info *types.Info, e ast.Expr) (string, bool) {
	if v := info.Types[e].Value; v != nil {
		if v.Kind() != constant.String {
			return "", false
		}
		return constant.StringVal(v), true
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		x, ok := k.known(info, e.X)
		if !ok {
			return "", false
		}
		y, ok := k.known(info, e.Y)
		return x + y, ok
	case *ast.Ident:
		return k.knownVar(info.Uses[e])
	case *ast.SelectorExpr:
		return k.knownVar(info.Uses[e.Sel])
	}
	return "", false
}

func (k *knownStrings) knownVar(obj types.Object) (string, bool) {
	v, ok := obj.(*types.Var)
	if !ok || v.Exported() || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() {
		return "", false
	}
	k.load(v.Pkg())
	kv := k.vars[v]
	if kv == nil || kv.written || kv.state == 1 {
		return "", false
	}
	if kv.state == 0 {
		kv.state = 1
		kv.val, kv.ok = k.known(kv.info, kv.init)
		kv.state = 2
	}
	return kv.val, kv.ok
}

// load finds the initializers of the package-level variables of tp and the
// variables its code assigns or takes the address of.
func (k *knownStrings) load(tp *types.Package) {
	if k.pkgs[tp] {
		return
	}
	if k.pkgs == nil {
		k.pkgs, k.vars = map[*types.Package]bool{}, map[*types.Var]*knownVar{}
	}
	k.pkgs[tp] = true
	pkg := k.byTypes[tp]
	if pkg == nil {
		return
	}
	info := pkg.TypesInfo
	get := func(id *ast.Ident) *knownVar {
		obj := info.Uses[id]
		if obj == nil {
			obj = info.Defs[id]
		}
		v, ok := obj.(*types.Var)
		if !ok || v.Pkg() != tp || v.Parent() != tp.Scope() {
			return nil
		}
		if k.vars[v] == nil {
			k.vars[v] = &knownVar{info: info}
		}
		return k.vars[v]
	}
	written := func(e ast.Expr) {
		switch e := ast.Unparen(e).(type) {
		case *ast.Ident:
			if kv := get(e); kv != nil {
				kv.written = true
			}
		case *ast.SelectorExpr:
			if kv := get(e.Sel); kv != nil {
				kv.written = true
			}
		}
	}
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						kv := get(name)
						if kv == nil {
							continue
						}
						if len(vs.Values) == len(vs.Names) {
							kv.init = vs.Values[i]
						} else {
							kv.written = true // a multi-value initializer
						}
					}
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if n.Tok != token.DEFINE {
					for _, l := range n.Lhs {
						written(l)
					}
				}
			case *ast.IncDecStmt:
				written(n.X)
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					written(n.X)
				}
			case *ast.RangeStmt:
				if n.Tok == token.ASSIGN {
					written(n.Key)
					if n.Value != nil {
						written(n.Value)
					}
				}
			}
			return true
		})
	}
	for _, kv := range k.vars {
		if kv.init == nil {
			kv.written = true // declared without a value, or in another way
		}
	}
}

// jsBoundaryIfaces reports whether interface values, errors among them, may
// cross the JavaScript boundary, where the runtime calls their Error method
// (GoError, goToJS): whether a package has a //goesm:import, or an exported
// function or method of the entry package has a parameter or result whose
// type holds an interface.
func (p *Program) jsBoundaryIfaces() bool {
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			for _, d := range f.Decls {
				var docs []*ast.CommentGroup
				switch d := d.(type) {
				case *ast.FuncDecl:
					docs = append(docs, d.Doc)
				case *ast.GenDecl:
					docs = append(docs, d.Doc)
					for _, s := range d.Specs {
						if vs, ok := s.(*ast.ValueSpec); ok {
							docs = append(docs, vs.Doc)
						}
					}
				}
				for _, doc := range docs {
					if d, _, err := jsImportDirective(doc); d != nil || err != nil {
						return true
					}
				}
			}
		}
	}
	var entry *packages.Package
	for _, pkg := range p.Pkgs {
		if pkg.PkgPath == p.Entry {
			entry = pkg
		}
	}
	if entry == nil {
		return true
	}
	seen := map[types.Type]bool{}
	var holds func(t types.Type) bool
	holds = func(t types.Type) bool {
		t = types.Unalias(t)
		if seen[t] {
			return false
		}
		seen[t] = true
		switch u := t.Underlying().(type) {
		case *types.Interface:
			return !isJSValue(t)
		case *types.Pointer:
			return holds(u.Elem())
		case *types.Slice:
			return holds(u.Elem())
		case *types.Array:
			return holds(u.Elem())
		case *types.Chan:
			return holds(u.Elem())
		case *types.Map:
			return holds(u.Key()) || holds(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if holds(u.Field(i).Type()) {
					return true
				}
			}
		case *types.Signature:
			return holdsSig(u, holds)
		}
		return false
	}
	scope := entry.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Func:
			if holdsSig(obj.Signature(), holds) {
				return true
			}
		case *types.TypeName:
			ms := types.NewMethodSet(types.NewPointer(obj.Type()))
			for i := 0; i < ms.Len(); i++ {
				if fn := ms.At(i).Obj(); fn.Exported() && holdsSig(fn.Type().(*types.Signature), holds) {
					return true
				}
			}
		}
	}
	return false
}

func holdsSig(sig *types.Signature, holds func(types.Type) bool) bool {
	for _, tup := range []*types.Tuple{sig.Params(), sig.Results()} {
		for i := 0; i < tup.Len(); i++ {
			if holds(tup.At(i).Type()) {
				return true
			}
		}
	}
	return false
}

// recovered reports whether the panic value e, in body, is what recover
// returned: a call of recover, or a variable that only such calls assign.
// Panicking with it again needs no other methods than the first panic.
func recovered(info *types.Info, body ast.Node, e ast.Expr) bool {
	isRecover := func(e ast.Expr) bool {
		call, ok := ast.Unparen(e).(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := ast.Unparen(call.Fun).(*ast.Ident)
		if !ok {
			return false
		}
		bi, ok := info.Uses[id].(*types.Builtin)
		return ok && bi.Name() == "recover"
	}
	if isRecover(e) {
		return true
	}
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return false
	}
	v, ok := info.Uses[id].(*types.Var)
	if !ok || v.Parent() == nil || v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
		return false
	}
	is := func(e ast.Expr) bool {
		id, ok := ast.Unparen(e).(*ast.Ident)
		return ok && (info.Uses[id] == v || info.Defs[id] == v)
	}
	only := true
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				if is(l) && (len(n.Rhs) != len(n.Lhs) || !isRecover(n.Rhs[i])) {
					only = false
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if is(name) && i < len(n.Values) && (len(n.Values) != len(n.Names) || !isRecover(n.Values[i])) {
					only = false
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND && is(n.X) {
				only = false
			}
		case *ast.RangeStmt:
			if is(n.Key) || n.Value != nil && is(n.Value) {
				only = false
			}
		}
		return only
	})
	return only
}
