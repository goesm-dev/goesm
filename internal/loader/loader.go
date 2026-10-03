// Package loader is goesm's Go frontend. It does no parsing, module
// resolution or type checking of its own: everything is delegated to the go
// command (via golang.org/x/tools/go/packages) and to go/types, so goesm accepts
// exactly the Go that the current toolchain accepts, with the module semantics
// (go.mod, go.sum, go.work, GOPROXY, ...) the go command implements.
package loader

import (
	"bytes"
	"fmt"
	"go/token"
	"go/version"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Target build constraints. GOOS=js GOARCH=wasm is the existing Go port that
// matches a JS host most closely: it selects js-specific files in the
// standard library and gives go/types the 64-bit wasm sizes.
var TargetEnv = []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}

// Program is a loaded, type-checked package graph.
type Program struct {
	Fset *token.FileSet
	// Roots are the packages named on the command line.
	Roots []*packages.Package
	// All packages in dependency order (dependencies first).
	All []*packages.Package
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
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
		Dir:  dir,
		Env:  append(os.Environ(), TargetEnv...),
		Fset: fset,
	}
	roots, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}

	var diags []Diagnostic
	var all []*packages.Package
	packages.Visit(roots, nil, func(p *packages.Package) {
		all = append(all, p) // post-order: dependencies first
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
	})
	if len(diags) > 0 {
		sort.SliceStable(diags, func(i, j int) bool { return diags[i].Pos < diags[j].Pos })
		if hint := VersionHint(dir); hint != "" {
			diags = append(diags, Diagnostic{Layer: "goesm", Msg: hint})
		}
		return nil, &Error{Diags: diags}
	}
	return &Program{Fset: fset, Roots: roots, All: all}, nil
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
