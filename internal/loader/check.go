package loader

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/tools/go/packages"
)

// loadMem loads patterns like packages.Load in LoadOverlay's mode, with mem
// applied by goesm (see overlay.go). The go command lists the packages; goesm
// parses and type-checks them itself, the way go/packages does, because the
// files mem adds may import packages the go command did not list for the
// package (go/packages' importer only resolves the listed imports).
func loadMem(cfg *packages.Config, mem *memOverlay, root string, patterns []string) ([]*packages.Package, error) {
	// The packages the added and replaced files import are loaded too.
	extra := map[string]bool{}
	for name, data := range mem.files {
		f, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ImportsOnly)
		if err != nil {
			continue // reported when the file is parsed
		}
		for _, is := range f.Imports {
			if p, err := strconv.Unquote(is.Path.Value); err == nil && p != "unsafe" && p != "C" {
				extra[p] = true
			}
		}
	}
	asked := map[string]bool{}
	for _, p := range patterns {
		asked[p] = true
	}
	all := append([]string{}, patterns...)
	for p := range extra {
		if !asked[p] {
			all = append(all, p)
		}
	}
	sort.Strings(all[len(patterns):])

	meta := *cfg
	meta.Mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedImports | packages.NeedDeps | packages.NeedModule
	meta.ParseFile = nil
	listed, err := packages.Load(&meta, all...)
	if err != nil {
		return nil, err
	}
	var roots []*packages.Package
	for _, r := range listed {
		if !extra[r.PkgPath] || asked[r.PkgPath] {
			roots = append(roots, r)
		}
	}
	var pkgs []*packages.Package
	packages.Visit(listed, nil, func(p *packages.Package) { pkgs = append(pkgs, p) })
	byPath := map[string]*packages.Package{}
	for _, p := range pkgs {
		byPath[p.PkgPath] = p
	}

	// Parse.
	parse := replacingParser(root, mem)
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	parseErrs := map[*packages.Package][]error{}
	for _, p := range pkgs {
		if p.PkgPath == "unsafe" {
			continue
		}
		if len(p.CompiledGoFiles) > 0 {
			dir := filepath.Dir(p.CompiledGoFiles[0])
			p.CompiledGoFiles = append(p.CompiledGoFiles, mem.added[dir]...)
			p.GoFiles = append(p.GoFiles, mem.added[dir]...)
		}
		p.Syntax = make([]*ast.File, len(p.CompiledGoFiles))
		errs := make([]error, len(p.CompiledGoFiles))
		for i, name := range p.CompiledGoFiles {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				data, ok := mem.file(name)
				if !ok {
					data, ok = cfg.Overlay[name]
				}
				if !ok {
					var err error
					if data, err = os.ReadFile(name); err != nil {
						errs[i] = err
						return
					}
				}
				p.Syntax[i], errs[i] = parse(cfg.Fset, name, data)
			}()
		}
		parseErrs[p] = errs
	}
	wg.Wait()
	for _, p := range pkgs {
		var files []*ast.File
		for i, f := range p.Syntax {
			if err := parseErrs[p][i]; err != nil {
				appendParseError(p, err)
			}
			if f != nil {
				files = append(files, f)
			}
		}
		p.Syntax = files
	}

	// The imports of the files as parsed: those the go command did not
	// list are resolved by package path.
	for _, p := range pkgs {
		for _, f := range p.Syntax {
			for _, is := range f.Imports {
				path, err := strconv.Unquote(is.Path.Value)
				if err != nil || path == "unsafe" || path == "C" || p.Imports[path] != nil {
					continue
				}
				dep := byPath[path]
				if dep == nil {
					dep = byPath["vendor/"+path]
				}
				if dep != nil {
					if p.Imports == nil {
						p.Imports = map[string]*packages.Package{}
					}
					p.Imports[path] = dep
				}
			}
		}
	}
	if cycle := importCycle(pkgs); cycle != "" {
		return nil, fmt.Errorf("import cycle not allowed (after -toolexec rewrites): %s", cycle)
	}

	// Type-check, dependencies first.
	sizes := types.SizesFor("gc", "wasm")
	done := map[*packages.Package]chan struct{}{}
	for _, p := range pkgs {
		done[p] = make(chan struct{})
	}
	for _, p := range pkgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done[p])
			for _, dep := range p.Imports {
				<-done[dep]
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			check(p, cfg.Fset, sizes)
		}()
	}
	wg.Wait()
	return roots, nil
}

// check type-checks p as go/packages does.
func check(p *packages.Package, fset *token.FileSet, sizes types.Sizes) {
	p.Fset = fset
	p.TypesSizes = sizes
	p.TypesInfo = &types.Info{
		Types:        map[ast.Expr]types.TypeAndValue{},
		Defs:         map[*ast.Ident]types.Object{},
		Uses:         map[*ast.Ident]types.Object{},
		Implicits:    map[ast.Node]types.Object{},
		Instances:    map[*ast.Ident]types.Instance{},
		Scopes:       map[ast.Node]*types.Scope{},
		Selections:   map[*ast.SelectorExpr]*types.Selection{},
		FileVersions: map[*ast.File]string{},
	}
	if p.PkgPath == "unsafe" {
		p.Types = types.Unsafe
		p.Syntax = []*ast.File{}
		return
	}
	p.Types = types.NewPackage(p.PkgPath, p.Name)
	tc := &types.Config{
		Importer: importerFunc(func(path string) (*types.Package, error) {
			if path == "unsafe" {
				return types.Unsafe, nil
			}
			dep := p.Imports[path]
			if dep == nil {
				return nil, fmt.Errorf("no metadata for %s", path)
			}
			if dep.Types == nil || !dep.Types.Complete() {
				return nil, fmt.Errorf("could not import %s (type-checking failed)", path)
			}
			return dep.Types, nil
		}),
		Error: func(err error) {
			var te types.Error
			if errors.As(err, &te) {
				p.Errors = append(p.Errors, packages.Error{Pos: fset.Position(te.Pos).String(), Msg: te.Msg, Kind: packages.TypeError})
				return
			}
			p.Errors = append(p.Errors, packages.Error{Pos: "-", Msg: err.Error(), Kind: packages.UnknownError})
		},
		Sizes: sizes,
	}
	if p.Module != nil && p.Module.GoVersion != "" {
		tc.GoVersion = "go" + p.Module.GoVersion
	}
	err := types.NewChecker(tc, fset, p.Types, p.TypesInfo).Files(p.Syntax)
	if err != nil && len(p.Errors) == 0 {
		p.Errors = append(p.Errors, packages.Error{Pos: "-", Msg: err.Error(), Kind: packages.TypeError})
	}
	p.IllTyped = len(p.Errors) > 0
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func appendParseError(p *packages.Package, err error) {
	var list scanner.ErrorList
	if errors.As(err, &list) {
		for _, e := range list {
			p.Errors = append(p.Errors, packages.Error{Pos: e.Pos.String(), Msg: e.Msg, Kind: packages.ParseError})
		}
		return
	}
	p.Errors = append(p.Errors, packages.Error{Pos: "-", Msg: err.Error(), Kind: packages.ParseError})
}

// importCycle returns a description of an import cycle among pkgs, or "".
func importCycle(pkgs []*packages.Package) string {
	const (
		visiting = 1
		visited  = 2
	)
	state := map[*packages.Package]int{}
	var stack []string
	var visit func(p *packages.Package) string
	visit = func(p *packages.Package) string {
		switch state[p] {
		case visiting:
			return fmt.Sprint(append(stack, p.PkgPath))
		case visited:
			return ""
		}
		state[p] = visiting
		stack = append(stack, p.PkgPath)
		paths := make([]string, 0, len(p.Imports))
		for path := range p.Imports {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			if c := visit(p.Imports[path]); c != "" {
				return c
			}
		}
		stack = stack[:len(stack)-1]
		state[p] = visited
		return ""
	}
	for _, p := range pkgs {
		if c := visit(p); c != "" {
			return c
		}
	}
	return ""
}
