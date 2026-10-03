package lower

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/goesm-dev/goesm/internal/sourcemap"
)

// Layout of the generated TypeScript: the module of Go package p is the file
// p + ".ts" below the output root (example.com/app/cart.ts, strings.ts), and
// the runtime is @goesm/runtime/index.ts (natives: natives.ts) next to them.
// Modules import each other with relative specifiers ending in .ts, which
// TypeScript (allowImportingTsExtensions), Vite, Rolldown, esbuild and Bun
// resolve without configuration. A Go import path cannot start with "@", so
// the runtime never collides with a Go package.
const (
	RuntimeFile = "@goesm/runtime/index.ts"
	NativesFile = "@goesm/runtime/natives.ts"
)

// ModuleFile is the path of the module of a Go package below the root.
func ModuleFile(importPath string) string { return importPath + ".ts" }

// relSpecifier is the specifier for importing file `to` from the module of
// Go package `from` (both relative to the output root).
func relSpecifier(from, to string) string {
	dir := strings.Split(path.Dir(ModuleFile(from)), "/")
	if dir[0] == "." {
		dir = nil
	}
	target := strings.Split(to, "/")
	i := 0
	for i < len(dir) && i < len(target)-1 && dir[i] == target[i] {
		i++
	}
	parts := []string{"."}
	for range dir[i:] {
		parts = append(parts, "..")
	}
	if len(parts) > 1 {
		parts = parts[1:]
	}
	return strings.Join(append(parts, target[i:]...), "/")
}

// Module is the lowering result for one Go package.
type Module struct {
	Path   string // Go import path
	TS     string // TypeScript source including an inline source map
	Map    []byte // generated TS -> .go source map (also inlined in TS)
	Native bool   // false: nothing to emit (e.g. package unsafe)
}

// Options control lowering.
type Options struct {
	// Entry is the import path whose `func main`, if present, runs when the
	// module is evaluated.
	Entry string
}

// LowerAll lowers every package of the program, dependencies first.
func (p *Program) LowerAll(opts Options) []*Module {
	var out []*Module
	for _, pkg := range p.Pkgs {
		if pkg.PkgPath == "unsafe" {
			continue
		}
		pe := newPkgEmitter(p, pkg, pkg.PkgPath == opts.Entry)
		out = append(out, pe.emit())
	}
	return out
}

type pkgEmitter struct {
	prog    *Program
	pkg     *packages.Package
	info    *types.Info
	tab     *posTable
	isEntry bool

	reserved    map[string]bool // package-level JS names
	imports     map[*types.Package]string
	importOrder []*types.Package

	typeConsts  typeutil.Map // types.Type -> hoisted descriptor const name
	anonStructs typeutil.Map // *types.Struct -> class name
	localTypes  map[*types.TypeName]string
	counter     int

	classes, phase1, consts, phase2, funcs, vars *writer
	exports                                      [][2]string // local, exported
	exportSet                                    map[string]bool

	lastPos token.Pos

	// std is set for standard library packages, whose functions that goesm
	// cannot lower yet become stubs that panic when called (a warning, not
	// an error: most programs never reach them).
	std bool
	// usesNatives: the module imports the runtime's natives ($natives).
	usesNatives bool

	inits    []string
	initObjs []any
}

func newPkgEmitter(p *Program, pkg *packages.Package, entry bool) *pkgEmitter {
	tab := &posTable{}
	pe := &pkgEmitter{
		prog:       p,
		pkg:        pkg,
		info:       pkg.TypesInfo,
		tab:        tab,
		isEntry:    entry,
		reserved:   map[string]bool{"$rt": true, "$natives": true},
		imports:    map[*types.Package]string{},
		localTypes: map[*types.TypeName]string{},
		classes:    newWriter(tab),
		phase1:     newWriter(tab),
		consts:     newWriter(tab),
		phase2:     newWriter(tab),
		funcs:      newWriter(tab),
		vars:       newWriter(tab),
		exportSet:  map[string]bool{},
		std:        p.std[pkg],
	}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		pe.reserved[jsName(name)] = true
	}
	return pe
}

// emitStdFuncDecl lowers a standard library function. If goesm cannot lower
// it yet, the diagnostics become one warning and the function a stub that
// panics when called.
func (pe *pkgEmitter) emitStdFuncDecl(file *ast.File, fd *ast.FuncDecl) {
	n := len(pe.prog.Diags)
	funcs := pe.funcs
	pe.funcs = newWriter(pe.tab)
	pe.emitFuncDecl(file, fd)
	body := pe.funcs
	pe.funcs = funcs
	if len(pe.prog.Diags) == n {
		funcs.append(body)
		return
	}
	first := pe.prog.Diags[n]
	pe.prog.Diags = pe.prog.Diags[:n]
	fn := pe.info.Defs[fd.Name].(*types.Func)
	pe.prog.Warns = append(pe.prog.Warns, Diagnostic{
		Pos: pe.prog.Fset.Position(fd.Pos()),
		Msg: fmt.Sprintf("%s is not lowered and panics if called: %s", fn.FullName(), first.Msg),
	})
	name := pe.funcDeclName(fd, fn)
	if fd.Name.Name == "init" {
		name = pe.inits[len(pe.inits)-1]
	}
	funcs.ln("%sfunction %s(...a: any[]): any { $rt.plainPanic(%s); }", pe.tab.mark(fd.Pos()), name, jsString("goesm: "+fn.FullName()+" is not supported yet"))
}

func (pe *pkgEmitter) errorf(pos token.Pos, format string, args ...any) {
	if !pos.IsValid() {
		pos = pe.lastPos // best known location for position-less constructs
	}
	pe.prog.errorf(pos, format, args...)
}

func (pe *pkgEmitter) export(local, name string) {
	if pe.exportSet[name] {
		return
	}
	pe.exportSet[name] = true
	pe.exports = append(pe.exports, [2]string{local, name})
}

// importAlias returns the JS namespace name for an imported Go package.
func (pe *pkgEmitter) importAlias(p *types.Package) string {
	if a, ok := pe.imports[p]; ok {
		return a
	}
	base := jsName(p.Name())
	a := base
	for i := 2; pe.reserved[a]; i++ {
		a = fmt.Sprintf("%s$%d", base, i)
	}
	pe.reserved[a] = true
	pe.imports[p] = a
	pe.importOrder = append(pe.importOrder, p)
	return a
}

// qualify returns the JS reference to a package-level name of package p.
func (pe *pkgEmitter) qualify(p *types.Package, name string) string {
	if p == nil || p == pe.pkg.Types {
		return name
	}
	return pe.importAlias(p) + "." + name
}

func (pe *pkgEmitter) fresh(prefix string) string {
	pe.counter++
	return fmt.Sprintf("$%s%d", prefix, pe.counter)
}

// namedTypeName returns the JS base name of a defined type (package-level or
// hoisted local type).
func (pe *pkgEmitter) namedTypeName(tn *types.TypeName) string {
	if n, ok := pe.localTypes[tn]; ok {
		return n
	}
	if tn.Parent() != nil && tn.Pkg() != nil && tn.Parent() != tn.Pkg().Scope() {
		// Local type from another package cannot be referenced; local types of
		// this package are registered before emission.
		n := jsName(tn.Name()) + "$L" + fmt.Sprint(len(pe.localTypes)+1)
		pe.localTypes[tn] = n
		return n
	}
	return jsName(tn.Name())
}

func (pe *pkgEmitter) emit() *Module {
	pkg := pe.pkg
	files := append([]*ast.File(nil), pkg.Syntax...)
	sort.Slice(files, func(i, j int) bool {
		return pe.prog.Fset.Position(files[i].Pos()).Filename < pe.prog.Fset.Position(files[j].Pos()).Filename
	})

	// Defined types: package-level ones and hoisted local ones. Local types
	// are hoisted to module scope so each declaration has one identity.
	var typeNames []*types.TypeName
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		if tn, ok := scope.Lookup(name).(*types.TypeName); ok && !tn.IsAlias() {
			typeNames = append(typeNames, tn)
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok {
					if tn, ok := pe.info.Defs[ts.Name].(*types.TypeName); ok && !tn.IsAlias() {
						if hasTypeParam(tn.Type().Underlying()) {
							pe.errorf(ts.Pos(), "local type %s depending on type parameters is not supported yet", tn.Name())
							return true
						}
						name := jsName(tn.Name()) + "$L"
						for i := 1; ; i++ {
							c := fmt.Sprintf("%s%d", name, i)
							if !pe.reserved[c] {
								name = c
								break
							}
						}
						pe.reserved[name] = true
						pe.localTypes[tn] = name
						typeNames = append(typeNames, tn)
					}
				}
				return true
			})
			return false
		})
	}
	for _, tn := range typeNames {
		pe.emitNamedType(tn)
	}

	// Functions and methods.
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				if pe.std {
					pe.emitStdFuncDecl(f, fd)
				} else {
					pe.emitFuncDecl(f, fd)
				}
			}
		}
	}

	// Package variables: declare with zero values, then initialise in the
	// dependency order computed by go/types (types.Info.InitOrder).
	pe.emitVars(files)

	// Exported constants are useful to JS consumers; Go code itself sees
	// constants folded by go/types.
	for _, name := range scope.Names() {
		if c, ok := scope.Lookup(name).(*types.Const); ok && c.Exported() {
			if b, ok := c.Type().Underlying().(*types.Basic); ok && b.Kind() != types.UntypedNil {
				local := jsName(name)
				pe.vars.ln("const %s = %s;", local, constLit(c.Val(), c.Type()))
				pe.export(local, name)
			}
		}
	}

	// init functions in source order, then main for the entry package.
	for i, f := range pe.inits {
		call := f + "()"
		if pe.prog.async[pe.initObjs[i]] {
			call = "await " + call
		}
		pe.vars.ln("%s;", call)
	}
	if pe.isEntry && pkg.Name == "main" {
		if _, ok := scope.Lookup("main").(*types.Func); ok {
			pe.vars.ln("$rt.runMain(main);")
		}
	}

	// Reflection-ready metadata about exported functions (used by the golden
	// test harness and future JS ABI work).
	var meta []string
	for _, name := range scope.Names() {
		if pe.std {
			break
		}
		if fn, ok := scope.Lookup(name).(*types.Func); ok && fn.Exported() && fn.Signature().TypeParams().Len() == 0 {
			meta = append(meta, fmt.Sprintf("%s: { fn: %s, type: %s, async: %v }", jsPropName(name), jsName(name), pe.typeDesc(fn.Type(), tpScope{}), pe.prog.IsAsync(fn)))
		}
	}
	pe.vars.ln("const $goesm = { path: %s, funcs: { %s } };", jsString(pkg.PkgPath), strings.Join(meta, ", "))
	pe.export("$goesm", "$goesm")

	// Assemble.
	out := newWriter(pe.tab)
	out.ln("// Code generated by goesm from Go package %s. DO NOT EDIT.", pkg.PkgPath)
	out.ln("// Go semantics were checked by go/types and lowered by goesm; any ESM")
	out.ln("// bundler (or TypeScript-aware runtime) can consume this module.")
	out.ln("import * as $rt from %s;", jsString(relSpecifier(pkg.PkgPath, RuntimeFile)))
	if pe.usesNatives {
		out.ln("import * as $natives from %s;", jsString(relSpecifier(pkg.PkgPath, NativesFile)))
	}
	// Evaluate every dependency, not only referenced ones: a blank import, or
	// one used only through folded constants, must still run its variable
	// initializers and init functions (Go orders them by import path). A
	// bare import is needed because bundlers drop unused TS namespace imports.
	var deps []string
	for _, ip := range pkg.Types.Imports() { // after goesm replacements
		if ip.Path() != "unsafe" {
			deps = append(deps, ip.Path())
		}
	}
	sort.Strings(deps)
	for _, path := range deps {
		out.ln("import %s;", jsString(relSpecifier(pkg.PkgPath, ModuleFile(path))))
	}
	for _, ip := range pe.importOrder {
		out.ln("import * as %s from %s;", pe.imports[ip], jsString(relSpecifier(pkg.PkgPath, ModuleFile(ip.Path()))))
	}
	if pe.isEntry {
		out.ln("export * as $runtime from %s;", jsString(relSpecifier(pkg.PkgPath, RuntimeFile)))
	}
	for _, sec := range []*writer{pe.classes, pe.phase1, pe.consts, pe.phase2, pe.funcs, pe.vars} {
		out.append(sec)
	}
	if len(pe.exports) > 0 {
		var parts []string
		for _, e := range pe.exports {
			if e[0] == e[1] {
				parts = append(parts, e[0])
			} else {
				parts = append(parts, fmt.Sprintf("%s as %s", e[0], jsString(e[1])))
			}
		}
		out.ln("export { %s };", strings.Join(parts, ", "))
	}

	sm := sourcemap.NewBuilder()
	for _, m := range out.maps {
		pos := pe.prog.Fset.Position(m.pos)
		if pos.Filename == "" {
			continue
		}
		idx, ok := sm.SourceIndex(pos.Filename)
		if !ok {
			content, _ := os.ReadFile(pos.Filename)
			idx = sm.AddSource(pos.Filename, string(content))
		}
		sm.Add(sourcemap.Mapping{GenLine: m.line, GenCol: m.col, Source: idx, SrcLine: pos.Line - 1, SrcCol: sm.UTF16Col(idx, pos.Line, pos.Column)})
	}
	file := modFileName(pkg.PkgPath)
	ts := out.String()
	return &Module{
		Path:   pkg.PkgPath,
		TS:     ts + sm.InlineComment(file),
		Map:    sm.JSON(file),
		Native: true,
	}
}

func modFileName(path string) string {
	return path[strings.LastIndex(path, "/")+1:] + ".ts"
}

// emitVars declares package variables and runs their initialisers.
func (pe *pkgEmitter) emitVars(files []*ast.File) {
	scope := pe.pkg.Types.Scope()
	fe := pe.newFuncEmitter(pe.vars, nil)
	for _, name := range scope.Names() {
		v, ok := scope.Lookup(name).(*types.Var)
		if !ok {
			continue
		}
		local := jsName(name)
		init := pe.zeroOf(v.Type(), tpScope{})
		if pe.prog.boxed[v] {
			init = "$rt.cell(" + init + ")"
		}
		pe.vars.ln("let %s: %s = %s;", local, pe.varTSType(v), init)
		if v.Exported() {
			pe.export(local, name)
		}
	}
	for _, in := range pe.info.InitOrder {
		mark := pe.tab.mark(in.Rhs.Pos())
		if len(in.Lhs) == 1 {
			v := in.Lhs[0]
			rhs := fe.valueOf(in.Rhs, v.Type())
			if v.Name() == "_" {
				fe.discard(mark, rhs)
			} else {
				pe.vars.ln("%s%s = %s;", mark, fe.varRef(v), rhs)
			}
			continue
		}
		// f() or a comma-ok form (v, ok = m[k], <-ch, x.(T)).
		e, tt, ok := fe.commaOk(in.Rhs)
		if !ok {
			e, tt = fe.expr(in.Rhs), fe.info.TypeOf(in.Rhs)
		}
		t := fe.tmp()
		pe.vars.ln("%sconst %s = %s;", mark, t, e)
		for i, v := range in.Lhs {
			if v.Name() == "_" {
				continue
			}
			pe.vars.ln("%s = %s;", fe.varRef(v), fe.convertCopy(fmt.Sprintf("%s[%d]", t, i), tupleAt(tt, i), v.Type()))
		}
	}
}

func (pe *pkgEmitter) varTSType(v *types.Var) string {
	t := pe.tsType(v.Type(), tpScope{})
	if pe.prog.boxed[v] {
		return "$rt.Cell<" + t + ">"
	}
	return t
}

// constLit renders a Go constant of type t as a JS literal.
func constLit(v constant.Value, t types.Type) string {
	if isComplex(t) {
		re, _ := constant.Float64Val(constant.ToFloat(constant.Real(v)))
		im, _ := constant.Float64Val(constant.ToFloat(constant.Imag(v)))
		if isComplex64(t) {
			re32, _ := constant.Float32Val(constant.ToFloat(constant.Real(v)))
			im32, _ := constant.Float32Val(constant.ToFloat(constant.Imag(v)))
			re, im = float64(re32), float64(im32)
		}
		return "$rt.complex(" + formatFloat(re) + ", " + formatFloat(im) + ")"
	}
	if v.Kind() == constant.Complex { // a complex constant with a real type
		v = constant.Real(v)
	}
	switch v.Kind() {
	case constant.Bool:
		return fmt.Sprint(constant.BoolVal(v))
	case constant.String:
		return jsString(constant.StringVal(v))
	case constant.Int, constant.Float:
		isFloat := false
		if b, ok := t.Underlying().(*types.Basic); ok && b.Info()&types.IsFloat != 0 {
			isFloat = true
		}
		var s string
		if !isFloat && v.Kind() == constant.Int {
			s = v.ExactString()
		} else {
			f, _ := constant.Float64Val(v)
			if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.Float32 {
				f32, _ := constant.Float32Val(v)
				f = float64(f32)
			}
			s = formatFloat(f)
		}
		if strings.HasPrefix(s, "-") {
			return "(" + s + ")"
		}
		return s
	}
	return "undefined /* unsupported constant */"
}
