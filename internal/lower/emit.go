package lower

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
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
	ProgramFile = "@goesm/runtime/program.ts"
	JSABIFile   = "@goesm/runtime/jsabi.ts"
)

// NoCheck heads every generated module and runtime file: the code is
// type-checked by goesm's own tests (TestTSC), and a project's tsconfig
// (noUnusedLocals, noFallthroughCasesInSwitch, an ES2017 target that has no
// BigInt literals) or linter should not re-check it. Importers still see the
// modules' types.
const NoCheck = "// @ts-nocheck\n/* eslint-disable */\n"

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
	// Files are the JS and TS files the module imports for //goesm:import
	// directives, by absolute path (see JSImportSpecifier).
	Files []string
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
		if m := p.LowerPackage(pkg, opts); m != nil {
			out = append(out, m)
		}
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
	direct      map[*types.Package]string // aliases of the Go source's imports
	importOrder []*types.Package

	typeConsts  typeutil.Map      // types.Type -> hoisted descriptor const name
	zeroConsts  map[string]string // zero value expression -> hoisted zero function
	anonStructs typeutil.Map      // *types.Struct -> class name
	localTypes  map[*types.TypeName]string
	localGen    map[*types.TypeName]int // gc's numbering of local types
	counter     int

	classes, phase1, consts, phase2, funcs, vars *writer
	definesTypes                                 bool        // phase1 has $rt.defined types
	exports                                      [][2]string // local, exported
	exportSet                                    map[string]bool
	wrappers                                     map[string]string        // function -> its exportWrapper
	withBody                                     map[*types.Func]bool     // the entry package's methods that get wrappers
	usesJSABI                                    bool                     // an export wrapper uses $jsabi
	jsMethods                                    []string                 // statements giving classes their JS methods ($jsm)
	noCopy                                       map[*types.TypeName]bool // uncopied, once computed

	lastPos token.Pos

	// std is set for standard library packages, whose functions that goesm
	// cannot lower yet become stubs that panic when called (a warning, not
	// an error: most programs never reach them).
	std bool
	// dep is set for packages of third-party modules, whose functions that
	// goesm cannot lower become stubs as in the standard library.
	dep bool
	// runsMain: the module is a program's main package and runs main.
	runsMain bool
	// usesNatives: the module imports the runtime's natives ($natives).
	usesNatives bool
	// usesIR: the module declares $ir, the receiver of interface calls
	// (funcEmitter.icall).
	usesIR bool
	// byteBools and byteBoolMakes: the local []bool variables of the
	// function being emitted that a Uint8Array backs, and their make
	// calls (see byteBools).
	byteBools     map[*types.Var]bool
	byteBoolMakes map[*ast.CallExpr]bool
	// inBounds: the index expressions of the function being emitted whose
	// index is in range by construction (see inBoundsIndices).
	inBounds map[*ast.IndexExpr]bool
	// scalar: the range value variables of the function being emitted
	// that are read field by field (see scalarRangeVars).
	scalar map[*types.Var][]int
	// split holds the int64 and uint64 locals of the function being
	// lowered that live in two int32 halves (split64.go).
	split map[*types.Var]bool
	// strBytes holds the []byte locals kept as strings, and strMarshal the
	// json.Marshal calls defining them (see json.go).
	strBytes   map[*types.Var]bool
	strMarshal map[*ast.CallExpr]bool
	// jsonEncs maps types to their generated JSON encoders, which
	// jsonFuncs holds (see jsonenc.go).
	jsonEncs  typeutil.Map
	jsonFuncs *writer

	inits    []string
	initObjs []any
	// pureFuncCache and funcDeclMap serve returnsPure.
	pureFuncCache map[*types.Func]bool
	funcDeclMap   map[*types.Func]*ast.FuncDecl

	// The imports of //goesm:import directives (jsimport.go): local name by
	// specifier and export, the modules imported, and the files imported by
	// absolute path.
	jsImportNames map[string]string
	jsModules     map[string]*jsModule
	jsModuleOrder []*jsModule
	jsFiles       []string
	jsFileSet     map[string]bool
}

func newPkgEmitter(p *Program, pkg *packages.Package, entry bool) *pkgEmitter {
	tab := &posTable{}
	pe := &pkgEmitter{
		prog:       p,
		pkg:        pkg,
		info:       pkg.TypesInfo,
		tab:        tab,
		isEntry:    entry,
		reserved:   map[string]bool{"$rt": true, "$natives": true, "$ir": true, "$jsabi": true},
		imports:    map[*types.Package]string{},
		localTypes: map[*types.TypeName]string{},
		zeroConsts: map[string]string{},
		localGen:   map[*types.TypeName]int{},
		classes:    newWriter(tab),
		phase1:     newWriter(tab),
		consts:     newWriter(tab),
		phase2:     newWriter(tab),
		funcs:      newWriter(tab),
		jsonFuncs:  newWriter(tab),
		vars:       newWriter(tab),
		exportSet:  map[string]bool{},
		wrappers:   map[string]string{},
		std:        p.std[pkg],
		dep:        p.Deps[pkg],

		jsImportNames: map[string]string{},
		jsModules:     map[string]*jsModule{},
		jsFileSet:     map[string]bool{},
	}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		pe.reserved[jsName(name)] = true
	}
	// The aliases of the packages the Go source imports are reserved up
	// front, so that no local name takes one before its package is first
	// referenced.
	pe.direct = map[*types.Package]string{}
	for _, ip := range pkg.Types.Imports() {
		base := jsName(ip.Name())
		a := base
		for i := 2; pe.reserved[a]; i++ {
			a = fmt.Sprintf("%s$%d", base, i)
		}
		pe.reserved[a] = true
		pe.direct[ip] = a
	}
	return pe
}

// emitStdFuncDecl lowers a standard library or dependency function. If goesm cannot lower
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
	a, ok := pe.direct[p]
	if ok {
		pe.imports[p] = a
		pe.importOrder = append(pe.importOrder, p)
		return a
	}
	// A package the Go source does not import (it is referenced for a type
	// descriptor, say) may be first needed after a local took its name:
	// "$pkg" keeps it apart from Go identifiers and their "$N" renamings.
	base := jsName(p.Name()) + "$pkg"
	a = base
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
	// Local types are those declared in a block (of a function or a
	// function literal); they are numbered across the package in source
	// order like gc's (see generic in runtime/src/types.ts).
	gen := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			b, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			ast.Inspect(b, func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok {
					if tn, ok := pe.info.Defs[ts.Name].(*types.TypeName); ok && !tn.IsAlias() {
						gen++
						pe.localGen[tn] = gen
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
	// The entry package's methods are called from JavaScript through export
	// wrappers, also as methods of their struct classes.
	pe.withBody = map[*types.Func]bool{}
	if pe.isEntry && !pe.std && !pe.dep {
		for _, f := range files {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Body != nil {
					if fn, ok := pe.info.Defs[fd.Name].(*types.Func); ok && fn.Exported() {
						pe.withBody[fn] = true
					}
				}
			}
		}
	}
	for _, tn := range typeNames {
		pe.emitNamedType(tn)
	}

	// Functions and methods.
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				if pe.std || pe.dep {
					pe.emitStdFuncDecl(f, fd)
				} else {
					pe.emitFuncDecl(f, fd)
				}
			}
		}
	}

	if pe.isEntry && !pe.std && !pe.dep {
		pe.emitImportedHandleMethods()
		pe.emitJSMethods()
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

	// Symbols this package provides to //go:linkname pulls, once its
	// variables are initialized (see linkname.go).
	for _, name := range scope.Names() {
		if fn, ok := scope.Lookup(name).(*types.Func); ok {
			if sym, ok := pe.prog.linkProvides[fn]; ok {
				call := fmt.Sprintf("$rt.linkProvide(%s, %s)", jsString(sym), jsName(name))
				if pe.prog.async[fn] {
					call = pe.prog.awaitMain(call)
				}
				pe.vars.ln("%s;", call)
			}
		}
	}

	// init functions in source order, then main for the entry package.
	for i, f := range pe.inits {
		call := f + "()"
		if pe.prog.async[pe.initObjs[i]] {
			call = pe.prog.awaitMain(call)
		}
		pe.vars.ln("%s;", call)
	}
	if pe.isEntry && pkg.Name == "main" {
		if _, ok := scope.Lookup("main").(*types.Func); ok {
			pe.vars.ln("$rt.runMain(main);")
			pe.runsMain = true
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
			f := jsName(name)
			meta = append(meta, fmt.Sprintf("%s: { fn: %s, type: %s, async: %v }", jsPropName(name), f, pe.typeDesc(fn.Type(), tpScope{}), pe.prog.IsAsync(fn)))
		}
	}
	// The entry package's table also carries the runtime's conversion of Go
	// values to JSON-shaped values, for the harness to compare results
	// with native Go's encoding/json.
	toJS := ""
	if pe.isEntry {
		toJS = ", toJS: $rt.toJS"
	}
	pe.vars.ln("const $goesm = { path: %s, funcs: { %s }%s };", jsString(pkg.PkgPath), strings.Join(meta, ", "), toJS)
	pe.export("$goesm", "$goesm")

	// Assemble.
	out := newWriter(pe.tab)
	out.ln("// Code generated by goesm from Go package %s. DO NOT EDIT.", pkg.PkgPath)
	out.ln("// Go semantics were checked by go/types and lowered by goesm; any ESM")
	out.ln("// bundler (or TypeScript-aware runtime) can consume this module.")
	out.ln("// @ts-nocheck")
	out.ln("/* eslint-disable */")
	out.ln("import * as $rt from %s;", jsString(relSpecifier(pkg.PkgPath, RuntimeFile)))
	if pe.usesNatives {
		out.ln("import * as $natives from %s;", jsString(relSpecifier(pkg.PkgPath, NativesFile)))
	}
	if len(pe.jsModuleOrder) > 0 || pe.usesJSABI {
		out.ln("import * as $jsabi from %s;", jsString(relSpecifier(pkg.PkgPath, JSABIFile)))
	}
	if pe.runsMain {
		// Before the dependencies: their initialization may panic.
		out.ln("import %s;", jsString(relSpecifier(pkg.PkgPath, ProgramFile)))
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
	for _, l := range pe.jsImportDecls() {
		out.ln("%s", l)
	}
	if pe.usesIR {
		out.ln("let $ir: any;")
	}
	for _, sec := range []*writer{pe.classes, pe.phase1, pe.consts, pe.phase2, pe.funcs, pe.jsonFuncs, pe.vars} {
		if sec == pe.phase2 && pe.definesTypes {
			out.ln("$rt.flushTypes(); // the defined types' underlying types and methods")
		}
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
		if pos.Filename == "" || pos.Line < 1 {
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
		Files:  pe.jsFiles,
	}
}

func modFileName(path string) string {
	return path[strings.LastIndex(path, "/")+1:] + ".ts"
}

// emitVars declares package variables and runs their initialisers.
func (pe *pkgEmitter) emitVars(files []*ast.File) {
	scope := pe.pkg.Types.Scope()
	fe := pe.newFuncEmitter(pe.vars, nil)
	embeds := embedDirectives(files, pe.info)
	// A variable whose initializer has no side effects is declared with it,
	// as a pure expression, so that bundlers drop the unused ones (most of
	// unicode's tables, for one).
	pure := map[*types.Var]bool{}
	for _, in := range pe.info.InitOrder {
		if len(in.Lhs) == 1 && in.Lhs[0].Name() != "_" && embeds[in.Lhs[0]] == nil && pe.pureExpr(in.Rhs) {
			pure[in.Lhs[0]] = true
		}
	}
	for _, name := range scope.Names() {
		v, ok := scope.Lookup(name).(*types.Var)
		if !ok {
			continue
		}
		local := jsName(name)
		if v.Exported() {
			pe.export(local, name)
		}
		if pure[v] {
			continue
		}
		init := pe.zeroOf(v.Type(), tpScope{})
		if pe.prog.boxed[v] {
			init = "$rt.cell(" + init + ")"
		}
		if !jsLiteral.MatchString(init) && init != "null" && init != "false" {
			// A zero value has no effect: a variable nothing uses (one of
			// internal/cpu's feature sets) is dropped with its types.
			init = "/* @__PURE__ */ (() => " + init + ")()"
		}
		pe.vars.ln("let %s: %s = %s;", local, pe.varTSType(v), init)
	}
	for _, name := range scope.Names() {
		v, ok := scope.Lookup(name).(*types.Var)
		if pats := embeds[v]; ok && pats != nil {
			dir := filepath.Dir(pe.prog.Fset.File(v.Pos()).Name())
			if init := pe.embedInit(v, dir, pats); init != "" {
				pe.vars.ln("%s%s = %s;", pe.tab.mark(v.Pos()), fe.varRef(v), init)
			}
		}
	}
	pe.emitJSImportVars(files, fe)
	for _, in := range pe.info.InitOrder {
		mark := pe.tab.mark(in.Rhs.Pos())
		if len(in.Lhs) == 1 {
			v := in.Lhs[0]
			rhs := fe.valueOf(in.Rhs, v.Type())
			if pure[v] {
				if pe.prog.boxed[v] {
					rhs = "$rt.cell(" + rhs + ")"
				}
				pe.vars.ln("%slet %s: %s = /* @__PURE__ */ (() => %s)();", mark, fe.nameOf(v), pe.varTSType(v), rhs)
			} else if v.Name() == "_" {
				if !pe.pureExpr(in.Rhs) { // var _ I = T{}: a compile-time check
					fe.discard(mark, rhs)
				}
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

// pureExpr reports whether evaluating the package-level initializer e has
// no side effects and cannot panic: constants, composite literals and their
// addresses, function literals, package-level variables and functions and
// their addresses, arithmetic that cannot panic, make and new with constant
// sizes, and calls of the functions of pureFuncs and of the package's
// functions that only return such an expression of their parameters,
// combined only that way.
func (pe *pkgEmitter) pureExpr(e ast.Expr) bool {
	return pe.pureExprIn(e, nil)
}

// pureExprIn is pureExpr in a function whose parameters params hold the
// values of pure expressions.
func (pe *pkgEmitter) pureExprIn(e ast.Expr, params map[types.Object]bool) bool {
	if tv, ok := pe.info.Types[e]; ok && tv.Value != nil {
		return true
	}
	pureExpr := func(e ast.Expr) bool { return pe.pureExprIn(e, params) }
	switch e := e.(type) {
	case *ast.ParenExpr:
		return pureExpr(e.X)
	case *ast.FuncLit:
		return true
	case *ast.Ident:
		return params[pe.info.Uses[e]] || pe.pkgLevel(pe.info.Uses[e])
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			if _, ok := pe.info.Uses[id].(*types.PkgName); ok {
				return pe.pkgLevel(pe.info.Uses[e.Sel])
			}
		}
		return false
	case *ast.UnaryExpr:
		switch e.Op {
		case token.AND:
			// &T{...}, or the address of a package-level variable.
			switch x := unparen(e.X).(type) {
			case *ast.CompositeLit:
				return pureExpr(x)
			case *ast.Ident:
				v, ok := pe.info.Uses[x].(*types.Var)
				return ok && pe.pkgLevel(v)
			case *ast.SelectorExpr:
				v, ok := pe.info.Uses[x.Sel].(*types.Var)
				return ok && pe.info.Selections[x] == nil && pe.pkgLevel(v)
			}
			return false
		case token.SUB, token.ADD, token.XOR, token.NOT:
			return pureExpr(e.X)
		}
		return false
	case *ast.BinaryExpr:
		switch e.Op {
		case token.QUO, token.REM:
			// A constant divisor other than 0 (integer division of the
			// most negative value by -1 does not panic either).
			if v := pe.info.Types[e.Y].Value; v == nil || constant.Sign(v) == 0 {
				return false
			}
		case token.SHL, token.SHR:
			if pe.info.Types[e.Y].Value == nil {
				return false // a negative count panics
			}
		case token.EQL, token.NEQ:
			// Comparing interfaces, or arrays and structs holding them,
			// panics on incomparable dynamic types.
			for _, x := range []ast.Expr{e.X, e.Y} {
				switch under(pe.info.TypeOf(x)).(type) {
				case *types.Basic, *types.Pointer, *types.Chan:
				default:
					return false
				}
			}
		}
		return pureExpr(e.X) && pureExpr(e.Y)
	case *ast.CompositeLit:
		if m, ok := under(pe.info.TypeOf(e)).(*types.Map); ok && types.IsInterface(m.Key()) {
			return false // a key of an incomparable dynamic type panics
		}
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if _, field := under(pe.info.TypeOf(e)).(*types.Struct); !field && !pureExpr(kv.Key) {
					return false
				}
				el = kv.Value
			}
			if !pureExpr(el) {
				return false
			}
		}
		return true
	case *ast.CallExpr:
		// A conversion to a type other than an array (from a slice, which
		// panics if too short).
		if tv, ok := pe.info.Types[e.Fun]; ok && tv.IsType() && len(e.Args) == 1 {
			switch under(tv.Type).(type) {
			case *types.Pointer:
				// (*T)(nil)
				return pe.info.Types[e.Args[0]].IsNil()
			case *types.Array:
				return false
			}
			return pureExpr(e.Args[0])
		}
		if id, ok := unparen(e.Fun).(*ast.Ident); ok {
			if b, ok := pe.info.Uses[id].(*types.Builtin); ok {
				// make and new with constant sizes, which the type
				// checker has checked.
				if b.Name() != "make" && b.Name() != "new" {
					return false
				}
				for _, a := range e.Args[1:] {
					if pe.info.Types[a].Value == nil {
						return false
					}
				}
				return true
			}
		}
		// Calls of the functions that only allocate, which the standard
		// library's error variables and type descriptors are made with.
		var fn *types.Func
		var recv ast.Expr
		switch f := unparen(e.Fun).(type) {
		case *ast.Ident:
			fn, _ = pe.info.Uses[f].(*types.Func)
		case *ast.SelectorExpr:
			fn, _ = pe.info.Uses[f.Sel].(*types.Func)
			if sel := pe.info.Selections[f]; sel != nil {
				recv = f.X
			}
		}
		if fn == nil || recv != nil && !pureExpr(recv) {
			return false
		}
		if !pureFuncs[fn.Origin().FullName()] && (recv != nil || !pe.returnsPure(fn)) {
			return false
		}
		if fn.Name() == "Elem" && !pe.typeOfElemType(recv) {
			return false
		}
		for _, a := range e.Args {
			if !pureExpr(a) {
				return false
			}
		}
		return true
	}
	return false
}

// returnsPure reports whether fn is a function of the package whose body
// only returns a pure expression (pureExprIn) of its parameters, such as
// io/fs's errInvalid, which returns oserror.ErrInvalid.
func (pe *pkgEmitter) returnsPure(fn *types.Func) bool {
	if fn.Pkg() != pe.pkg.Types || fn.Signature().Recv() != nil || fn.Signature().TypeParams().Len() > 0 {
		return false
	}
	if pe.pureFuncCache == nil {
		pe.pureFuncCache = map[*types.Func]bool{}
	}
	if r, ok := pe.pureFuncCache[fn]; ok {
		return r
	}
	pe.pureFuncCache[fn] = false // a recursive function is not pure
	fd := pe.funcDecls()[fn]
	if fd == nil || fd.Body == nil || len(fd.Body.List) != 1 {
		return false
	}
	ret, ok := fd.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	params := map[types.Object]bool{}
	for i := 0; i < fn.Signature().Params().Len(); i++ {
		params[fn.Signature().Params().At(i)] = true
	}
	r := pe.pureExprIn(ret.Results[0], params)
	pe.pureFuncCache[fn] = r
	return r
}

// funcDecls maps the package's functions to their declarations.
func (pe *pkgEmitter) funcDecls() map[*types.Func]*ast.FuncDecl {
	if pe.funcDeclMap == nil {
		pe.funcDeclMap = map[*types.Func]*ast.FuncDecl{}
		for _, f := range pe.pkg.Syntax {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					if fn, ok := pe.info.Defs[fd.Name].(*types.Func); ok {
						pe.funcDeclMap[fn] = fd
					}
				}
			}
		}
	}
	return pe.funcDeclMap
}

// typeOfElemType reports whether e is reflect.TypeOf(x) (or reflectlite's)
// for an x whose type has an element type, so that Elem does not panic.
func (pe *pkgEmitter) typeOfElemType(e ast.Expr) bool {
	call, ok := unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	switch under(pe.info.TypeOf(call.Args[0])).(type) {
	case *types.Pointer, *types.Slice, *types.Array, *types.Map, *types.Chan:
		return true
	}
	return false
}

// pureFuncs are functions without effects that cannot panic when their
// arguments are pure expressions (pureExpr), receivers included.
var pureFuncs = map[string]bool{
	"errors.New":                       true,
	"os.NewFile":                       true, // os.Stdin, Stdout and Stderr
	"os.runtime_args":                  true, // os.Args (see the os patch)
	"time.runtimeNano":                 true, // time.startNano
	"sync.OnceFunc":                    true,
	"sync.OnceValue":                   true,
	"sync.OnceValues":                  true,
	"internal/reflectlite.TypeOf":      true,
	"reflect.TypeOf":                   true,
	"(internal/reflectlite.Type).Elem": true, // of TypeOf((*T)(nil)) (typeOfElemType)
	"(reflect.Type).Elem":              true,
}

// pkgLevel reports whether obj is a package-level variable or a function.
func (pe *pkgEmitter) pkgLevel(obj types.Object) bool {
	switch obj := obj.(type) {
	case *types.Func:
		return true
	case *types.Var:
		return obj.Parent() != nil && obj.Parent() == obj.Pkg().Scope()
	case *types.Nil:
		return true
	}
	return false
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
		if isBig(t) {
			s := constant.ToInt(v).ExactString() + "n"
			if strings.HasPrefix(s, "-") {
				return "(" + s + ")"
			}
			return s
		}
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
