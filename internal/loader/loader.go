// Package loader is goesm's Go frontend. It does no parsing, module
// resolution or type checking of its own: everything is delegated to the go
// command (via golang.org/x/tools/go/packages) and to go/types, so goesm accepts
// exactly the Go that the current toolchain accepts, with the module semantics
// (go.mod, go.sum, go.work, GOPROXY, ...) the go command implements.
package loader

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/goesm-dev/goesm/internal/natives"
	"golang.org/x/tools/go/packages"
)

// Target build constraints. GOOS=js GOARCH=wasm is the existing Go port that
// matches a JS host most closely: it selects js-specific files in the
// standard library and gives go/types the 64-bit wasm sizes.
var TargetEnv = []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}

// BuildTags select the standard library's portable Go implementations where
// it has assembly ones: purego (crypto and others) and math_big_pure_go.
// goesm cannot use assembly, and these build tags are also how gc builds
// these packages without it.
var BuildTags = []string{"purego", "math_big_pure_go"}

// Program is a loaded, type-checked package graph.
type Program struct {
	Fset *token.FileSet
	// Roots are the packages named on the command line.
	Roots []*packages.Package
	// All packages in dependency order (dependencies first).
	All []*packages.Package
	// Std is the set of standard library packages (sources in GOROOT).
	Std map[*packages.Package]bool
}

// Diagnostic is a frontend error reported at its original .go position.
type Diagnostic struct {
	Layer string // "go list", "go/parser", "go/types"
	Pos   string // file:line:col as reported by the Go frontend
	Msg   string
}

func (d Diagnostic) String() string {
	if d.Pos == "" || d.Pos == "-" {
		return fmt.Sprintf("%s [%s]", d.Msg, d.Layer)
	}
	return fmt.Sprintf("%s: %s [%s]", d.Pos, d.Msg, d.Layer)
}

// Error aggregates frontend diagnostics.
type Error struct{ Diags []Diagnostic }

func (e *Error) Error() string {
	var b strings.Builder
	for i, d := range e.Diags {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.String())
	}
	return b.String()
}

// Load loads the packages matching patterns (ordinary Go package patterns
// such as ./main or example.com/app/...) relative to dir.
func Load(dir string, patterns ...string) (*Program, error) {
	return LoadOverlay(dir, nil, patterns...)
}

// LoadOverlay is Load with file contents that replace or add files on disk,
// keyed by absolute path. It is the go command's own overlay mechanism
// (go build -overlay, packages.Config.Overlay): the go command still resolves
// packages and modules, it only reads these files from memory. Hosts that
// embed Go in other files (such as gosfc for Vue SFCs) use it to hand goesm a
// generated file without writing it into the user's source tree.
func LoadOverlay(dir string, overlay map[string][]byte, patterns ...string) (*Program, error) {
	fset := token.NewFileSet()
	root := goroot(dir)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
		Dir:        dir,
		Env:        append(os.Environ(), TargetEnv...),
		BuildFlags: []string{"-tags=" + strings.Join(BuildTags, ",")},
		Fset:       fset,
		ParseFile:  replacingParser(root),
		Overlay:    overlay,
	}
	roots, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}

	var all []*packages.Package
	packages.Visit(roots, nil, func(p *packages.Package) {
		all = append(all, p) // post-order: dependencies first
	})
	all = reachable(roots, all)
	var diags []Diagnostic
	for _, p := range all {
		for _, e := range p.Errors {
			layer := "go list"
			switch e.Kind {
			case packages.ParseError:
				layer = "go/parser"
			case packages.TypeError:
				layer = "go/types"
			}
			diags = append(diags, Diagnostic{Layer: layer, Pos: e.Pos, Msg: e.Msg})
		}
	}
	if len(diags) > 0 {
		sort.SliceStable(diags, func(i, j int) bool { return diags[i].Pos < diags[j].Pos })
		if hint := VersionHint(dir); hint != "" {
			diags = append(diags, Diagnostic{Layer: "goesm", Msg: hint})
		}
		return nil, &Error{Diags: diags}
	}
	std := map[*packages.Package]bool{}
	for _, p := range all {
		if root != "" && p.Module == nil && len(p.GoFiles) > 0 && strings.HasPrefix(p.GoFiles[0], filepath.Join(root, "src")+string(filepath.Separator)) {
			std[p] = true
		}
	}
	return &Program{Fset: fset, Roots: roots, All: all, Std: std}, nil
}

func goroot(dir string) string {
	cmd := exec.Command("go", "env", "GOROOT")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), TargetEnv...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// replacingParser parses Go files as go/packages would, except for the
// standard library packages that goesm replaces (internal/natives): the
// first file of such a package to be parsed becomes the replacement, the
// others become empty files (only their package clause is kept).
func replacingParser(goroot string) func(*token.FileSet, string, []byte) (*ast.File, error) {
	src := filepath.Join(goroot, "src") + string(filepath.Separator)
	var mu sync.Mutex
	replaced := map[string]bool{} // package dirs whose replacement was handed out
	return func(fset *token.FileSet, filename string, data []byte) (*ast.File, error) {
		const mode = parser.AllErrors | parser.ParseComments | parser.SkipObjectResolution
		if goroot == "" || !strings.HasPrefix(filename, src) {
			return parser.ParseFile(fset, filename, data, mode)
		}
		pkgDir := filepath.Dir(filename)
		importPath := filepath.ToSlash(strings.TrimPrefix(pkgDir, src))
		repl, ok := natives.Replacement(importPath)
		if !ok {
			f, err := parser.ParseFile(fset, filename, data, mode)
			if err != nil {
				return nil, err
			}
			if names := natives.Patched(importPath); names != nil {
				return patch(fset, f, names, importPath, filepath.Base(filename))
			}
			return f, nil
		}
		mu.Lock()
		first := !replaced[pkgDir]
		replaced[pkgDir] = true
		mu.Unlock()
		if first {
			return parser.ParseFile(fset, repl.Name, repl.Src, mode)
		}
		f, err := parser.ParseFile(fset, filename, data, parser.PackageClauseOnly)
		if err != nil {
			return nil, err
		}
		return &ast.File{Package: f.Package, Name: f.Name, FileStart: f.FileStart, FileEnd: f.FileEnd}, nil
	}
}

// patch applies the natives patches of package importPath to its file f
// (base is its file name). The declarations the patches replace are renamed
// to _, which keeps them type-checked (and their imports used) while nothing
// can refer to them, and goesm does not emit blank functions; a replaced
// variable's original initializer still runs. The patch of this file, if
// any, is added to it.
//
// A package may have several init functions, so a patch's init replaces only
// those of the file it patches.
func patch(fset *token.FileSet, f *ast.File, names map[string]bool, importPath, base string) (*ast.File, error) {
	var pf *ast.File
	if p, ok := natives.Patch(importPath, base); ok {
		var err error
		pf, err = parser.ParseFile(fset, p.Name, p.Src, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
	}
	patchesInit := false
	if pf != nil {
		for _, d := range pf.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "init" {
				patchesInit = true
			}
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			ns := natives.DeclNames(d)
			if d.Recv == nil && d.Name.Name == "init" {
				if patchesInit {
					d.Name = ast.NewIdent("_")
				}
			} else if len(ns) == 1 && names[ns[0]] {
				d.Name = ast.NewIdent("_")
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if names[s.Name.Name] {
						s.Name = ast.NewIdent("_")
					}
				case *ast.ValueSpec:
					for i, n := range s.Names {
						if names[n.Name] {
							s.Names[i] = ast.NewIdent("_")
						}
					}
				}
			}
		}
	}
	if pf == nil {
		return f, nil
	}
	have := map[string]bool{}
	for _, is := range f.Imports {
		have[importKey(is)] = true
	}
	var imports, rest []ast.Decl
	for _, d := range pf.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			var specs []ast.Spec
			for _, s := range g.Specs {
				if is := s.(*ast.ImportSpec); !have[importKey(is)] {
					specs = append(specs, is)
					f.Imports = append(f.Imports, is)
				}
			}
			if len(specs) > 0 {
				g.Specs = specs
				imports = append(imports, g)
			}
			continue
		}
		rest = append(rest, d)
	}
	f.Decls = append(append(imports, f.Decls...), rest...)
	return f, nil
}

func importKey(is *ast.ImportSpec) string {
	if is.Name != nil {
		return is.Name.Name + " " + is.Path.Value
	}
	return is.Path.Value
}

// reachable keeps the packages actually imported (by their syntax, after
// replacements) from the roots: a replaced package's original imports, such
// as the gc runtime's internals, are loaded by the go command but not built.
func reachable(roots, all []*packages.Package) []*packages.Package {
	keep := map[*packages.Package]bool{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if keep[p] {
			return
		}
		keep[p] = true
		if p.Types == nil {
			return
		}
		// p.Imports is keyed by the import path as written, which for a
		// vendored package (vendor/golang.org/x/...) is not its path.
		byPath := map[string]*packages.Package{}
		for _, dep := range p.Imports {
			byPath[dep.PkgPath] = dep
		}
		for _, ip := range p.Types.Imports() {
			if dep := byPath[ip.Path()]; dep != nil {
				visit(dep)
			}
		}
	}
	for _, r := range roots {
		visit(r)
	}
	var out []*packages.Package
	for _, p := range all {
		if keep[p] {
			out = append(out, p)
		}
	}
	return out
}

// VersionHint explains a frontend/toolchain skew. go/types is linked into the
// goesm binary, so the newest Go syntax goesm understands is that of the
// toolchain goesm was built with. goesm does not pin a Go version; running it
// via `go tool goesm` (a tool directive in go.mod) or `go run` rebuilds it with
// the toolchain the module selects.
func VersionHint(dir string) string {
	cmd := exec.Command("go", "env", "GOVERSION")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	tool := strings.TrimSpace(string(bytes.TrimSpace(out)))
	self := runtime.Version()
	if version.IsValid(tool) && version.IsValid(self) && version.Compare(version.Lang(tool), version.Lang(self)) > 0 {
		return fmt.Sprintf("goesm was built with %s but the go command selects %s; rebuild goesm with the current toolchain (e.g. `go tool goesm` with a tool directive) so its frontend understands %s syntax", self, tool, version.Lang(tool))
	}
	return ""
}
