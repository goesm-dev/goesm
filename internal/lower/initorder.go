package lower

import (
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// The order of package initialization across modules.
//
// Go initializes one package at a time. goesm initializes a package when
// its module is evaluated, and a package whose initialization blocks (an
// init function or a variable initializer that waits) awaits at the top
// level of its module. JavaScript then goes on evaluating the modules that
// do not import that one: in a bundle esbuild evaluates the modules one
// after the other, but with one module per package (goesm build -split,
// goesm emit-ts and the dev servers that load its modules), the next
// package's initialization ran while the first one waited. It then found
// the first one's state half built, or a mutex the first one held and that
// goesm lowered to one that does not wait, as nothing else runs during
// initialization in Go.
//
// So the module of a package imports the last package before it, in the
// order the modules are evaluated, whose initialization may block, unless
// it imports that package anyway: its evaluation then waits for that
// package's, which waits for the previous one's. The order is the one in
// which JavaScript evaluates the modules from the entries: depth first,
// each package after the packages it imports, in the order of their paths.

// InitAfter returns the import path of the package whose module the module
// of pkg imports so that pkg is initialized after it (see above), or "".
func (p *Program) InitAfter(pkg *packages.Package) string {
	p.initAfterOnce.Do(p.findInitAfter)
	return p.initAfter[pkg]
}

func (p *Program) findInitAfter() {
	p.initAfter = map[*packages.Package]string{}
	imports := func(pkg *packages.Package) []*packages.Package {
		var deps []*packages.Package
		for _, ip := range pkg.Types.Imports() {
			if d := p.byTypes[ip]; d != nil && ip.Path() != "unsafe" {
				deps = append(deps, d)
			}
		}
		sort.Slice(deps, func(i, j int) bool { return deps[i].PkgPath < deps[j].PkgPath })
		return deps
	}
	var order []*packages.Package
	seen := map[*packages.Package]bool{}
	var visit func(pkg *packages.Package)
	visit = func(pkg *packages.Package) {
		if seen[pkg] || pkg.Types == nil {
			return
		}
		seen[pkg] = true
		for _, d := range imports(pkg) {
			visit(d)
		}
		order = append(order, pkg)
	}
	var entries []*packages.Package
	for _, pkg := range p.Pkgs {
		if p.Entries[pkg.PkgPath] {
			entries = append(entries, pkg)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].PkgPath < entries[j].PkgPath })
	for _, pkg := range entries {
		visit(pkg)
	}
	// dependsOn reports whether pkg imports dep, directly or not.
	dependsOn := func(pkg, dep *packages.Package) bool {
		done := map[*packages.Package]bool{}
		var walk func(*packages.Package) bool
		walk = func(q *packages.Package) bool {
			if q == dep {
				return true
			}
			if done[q] {
				return false
			}
			done[q] = true
			for _, d := range imports(q) {
				if walk(d) {
					return true
				}
			}
			return false
		}
		return walk(pkg)
	}
	var last *packages.Package
	for _, pkg := range order {
		if last != nil && !dependsOn(pkg, last) {
			p.initAfter[pkg] = last.PkgPath
		}
		if p.initBlocks(pkg) {
			last = pkg
		}
	}
}

// initBlocks reports whether initializing pkg may block: an init function
// it calls, a variable initializer it evaluates or a function it provides
// to //go:linkname pulls may.
func (p *Program) initBlocks(pkg *packages.Package) bool {
	info := pkg.TypesInfo
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			fn, ok := info.Defs[fd.Name].(*types.Func)
			if !ok || !p.async[fn] {
				continue
			}
			if fd.Name.Name == "init" && p.InitCalled(fn) {
				return true
			}
			if _, ok := p.linkProvides[fn]; ok {
				return true
			}
		}
	}
	for _, in := range info.InitOrder {
		if v := initVar(in); v != nil && !p.VarReached(v, in.Rhs) {
			continue
		}
		if _, ok := p.blocksAt(info, in.Rhs); ok {
			return true
		}
	}
	return false
}
