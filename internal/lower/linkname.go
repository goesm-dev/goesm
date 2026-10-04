package lower

import (
	"go/ast"
	"go/types"
	"strings"

	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// //go:linkname between Go packages.
//
// A function declared without a body and a //go:linkname directive naming a
// function of another package (a "pull", as compile-time instrumentation
// like otelc generates to reach its hooks) calls that function. The call
// goes through a symbol table in @goesm/runtime, not an ES module import:
// the linked packages may import each other the other way round, and an
// import would change the order in which packages are initialized, which Go
// leaves to the import graph alone. The providing package registers its
// symbols once its package variables are initialized; a call that comes
// earlier and returns nothing is deferred until then (Go would run it at
// once, against the statically initialized variables), anything else
// panics.
//
// The standard library's own pulls into the runtime keep being implemented
// by goesm's natives.

type linkDirective struct {
	local, target string
}

// linkDirectives returns the //go:linkname directives of a file.
func linkDirectives(f *ast.File) []linkDirective {
	var ds []linkDirective
	for _, g := range f.Comments {
		for _, c := range g.List {
			rest, ok := strings.CutPrefix(c.Text, "//go:linkname ")
			if !ok {
				continue
			}
			fs := strings.Fields(rest)
			switch len(fs) {
			case 1:
				ds = append(ds, linkDirective{local: fs[0]})
			case 2:
				ds = append(ds, linkDirective{local: fs[0], target: fs[1]})
			}
		}
	}
	return ds
}

// linkSymbol is the linker symbol of a package-level function.
func linkSymbol(fn *types.Func) string {
	pkg := fn.Pkg()
	if pkg.Name() == "main" {
		return "main." + fn.Name()
	}
	return pkg.Path() + "." + fn.Name()
}

// analyzeLinknames resolves the pulls of the program to the functions that
// provide their symbols.
func (p *Program) analyzeLinknames() {
	p.linkPulls = map[*types.Func]string{}
	p.linkProvides = map[*types.Func]string{}
	p.linkTargets = map[*types.Func]*types.Func{}
	providers := map[string]*types.Func{}
	type pull struct {
		fn  *types.Func
		sym string
	}
	var pulls []pull
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			ds := linkDirectives(f)
			if len(ds) == 0 {
				continue
			}
			byName := map[string]string{}
			for _, d := range ds {
				byName[d.local] = d.target
			}
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv != nil {
					continue
				}
				target, ok := byName[fd.Name.Name]
				if !ok || target == "" {
					continue
				}
				fn, _ := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
				if fn == nil || fn.Signature().TypeParams().Len() > 0 {
					continue
				}
				if fd.Body != nil {
					providers[target] = fn // a push: fn is the symbol target
					continue
				}
				if p.std[pkg] && goesmruntime.HasNative(fn.FullName()) {
					continue
				}
				pulls = append(pulls, pull{fn, target})
			}
		}
	}
	if len(pulls) == 0 {
		return
	}
	for _, pkg := range p.Pkgs {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			if fn, ok := scope.Lookup(name).(*types.Func); ok && fn.Signature().TypeParams().Len() == 0 {
				sym := linkSymbol(fn)
				if providers[sym] == nil {
					providers[sym] = fn
				}
			}
		}
	}
	for _, pl := range pulls {
		target := providers[pl.sym]
		if target == nil || target == pl.fn {
			continue
		}
		if _, isNative := p.bodyless(target); isNative {
			continue // a pull of a pull: no Go body to call
		}
		p.linkPulls[pl.fn] = pl.sym
		p.linkProvides[target] = pl.sym
		p.linkTargets[pl.fn] = target
	}
}

// bodyless reports whether fn is declared without a body.
func (p *Program) bodyless(fn *types.Func) (*ast.FuncDecl, bool) {
	pkg := p.byTypes[fn.Pkg()]
	if pkg == nil {
		return nil, false
	}
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == fn.Name() {
				if pkg.TypesInfo.Defs[fd.Name] == fn {
					return fd, fd.Body == nil
				}
			}
		}
	}
	return nil, false
}
