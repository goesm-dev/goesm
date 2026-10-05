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
// String itself, and a program that enumerates methods by reflection may
// call any exported one.

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

// reach computes calledMethods and unicodeClasses.
func (p *Program) reach() {
	p.calledMethods = map[string][]ifaceMethod{}
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
		if b, ok := decls[fn]; ok && !seen[fn] {
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

	errorIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	called(errorIface.Method(0))
	for _, m := range p.ifaceMethods["String"] {
		if m.fn.Pkg() == nil { // the runtime's String() string (findDynMethods)
			called(m.fn)
		}
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
	for len(queue) > 0 {
		b := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		ast.Inspect(b.node, func(n ast.Node) bool {
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
