// Package build connects the pipeline: Go frontend (loader) -> semantic
// lowering (lower) -> TypeScript files -> esbuild (Go API) -> ES modules.
//
// esbuild owns TypeScript syntax stripping, JS printing, target lowering,
// bundling, tree shaking, minification, code splitting and final source map
// emission. It never sees Go and makes no Go-semantic decisions.
package build

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/goesm-dev/goesm/internal/loader"
	"github.com/goesm-dev/goesm/internal/lower"
	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// Options for a build.
type Options struct {
	Dir      string   // working directory (module root or below)
	Patterns []string // Go package patterns, e.g. ./main
	OutDir   string   // output directory (default "dist")
	TSDir    string   // where generated TypeScript is written ("" = temp dir)
	Split    bool     // one ES module per Go package instead of one bundle
	Minify   bool
	// Overlay replaces or adds files by absolute path, as go build -overlay
	// does. Nil means the files on disk.
	Overlay map[string][]byte
}

// Result describes the outputs.
type Result struct {
	Outputs []string // written JS files
	TSDir   string
}

// DiagError carries diagnostics from one pipeline layer.
type DiagError struct {
	Layer string
	Lines []string
}

func (e *DiagError) Error() string { return strings.Join(e.Lines, "\n") }

// Lower runs the Go frontend and the semantic lowering.
func Lower(dir string, patterns []string) ([]*lower.Module, string, error) {
	return LowerOverlay(dir, nil, patterns)
}

// LowerOverlay is Lower with an overlay (see Options.Overlay).
func LowerOverlay(dir string, overlay map[string][]byte, patterns []string) ([]*lower.Module, string, error) {
	prog, err := loader.LoadOverlay(dir, overlay, patterns...)
	if err != nil {
		if le, ok := err.(*loader.Error); ok {
			var lines []string
			for _, d := range le.Diags {
				lines = append(lines, d.String())
			}
			return nil, "", &DiagError{Layer: "go", Lines: lines}
		}
		return nil, "", err
	}
	if len(prog.Roots) != 1 {
		return nil, "", fmt.Errorf("goesm: expected exactly one package, got %d", len(prog.Roots))
	}
	entry := prog.Roots[0].PkgPath
	lp := lower.NewProgram(prog.Fset, prog.All)
	mods := lp.LowerAll(lower.Options{Entry: entry})
	if len(lp.Diags) > 0 {
		var lines []string
		for _, d := range lp.SortedDiags() {
			lines = append(lines, d.String())
		}
		return nil, "", &DiagError{Layer: "goesm", Lines: lines}
	}
	return mods, entry, nil
}

// ReadOverlay reads an overlay file in the go command's -overlay format,
// {"Replace": {"/abs/file.go": "/path/with/contents.go"}}, into file contents
// keyed by absolute path. Deleting files (an empty replacement) is not
// supported.
func ReadOverlay(file string) (map[string][]byte, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var o struct{ Replace map[string]string }
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("overlay %s: %v", file, err)
	}
	base := filepath.Dir(file)
	m := map[string][]byte{}
	for target, src := range o.Replace {
		if src == "" {
			return nil, fmt.Errorf("overlay %s: deleting %s is not supported", file, target)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(base, target)
		}
		if !filepath.IsAbs(src) {
			src = filepath.Join(base, src)
		}
		c, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		m[target] = c
	}
	return m, nil
}

// tsPath is where the module of a Go package is written below the TS dir.
func tsPath(tsDir, importPath string) string {
	return filepath.Join(tsDir, "go", filepath.FromSlash(importPath)+".ts")
}

// WriteTS writes generated modules and the embedded runtime to dir.
func WriteTS(dir string, mods []*lower.Module) error {
	for _, m := range mods {
		p := tsPath(dir, m.Path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(m.TS), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(p+".map", m.Map, 0o644); err != nil {
			return err
		}
	}
	return fs.WalkDir(goesmruntime.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := goesmruntime.Files.ReadFile(path)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, "@goesm", "runtime", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

// resolver maps "go:<import path>" and "@goesm/runtime" to generated files.
// It is goesm's own code; there is no third-party plugin mechanism.
//
// In split mode every Go package becomes its own ES module and imports
// between packages stay ES module imports (made relative to the output
// layout), so the Go package graph is the ES module graph.
func resolver(tsDir, outDir string, split bool) api.Plugin {
	outPath := func(importer string) string {
		// Output path of the module generated from TS file importer.
		rel, _ := filepath.Rel(filepath.Join(tsDir, "go"), importer)
		return filepath.Join(outDir, strings.TrimSuffix(rel, ".ts")+".js")
	}
	relImport := func(importer, target string) string {
		r, _ := filepath.Rel(filepath.Dir(outPath(importer)), target)
		r = filepath.ToSlash(r)
		if !strings.HasPrefix(r, ".") {
			r = "./" + r
		}
		return r
	}
	return api.Plugin{
		Name: "goesm",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: `^go:`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				path := strings.TrimPrefix(args.Path, "go:")
				p := tsPath(tsDir, path)
				if _, err := os.Stat(p); err != nil {
					return api.OnResolveResult{}, fmt.Errorf("Go package %s was not lowered", path)
				}
				if split {
					return api.OnResolveResult{Path: relImport(args.Importer, filepath.Join(outDir, filepath.FromSlash(path)+".js")), External: true}, nil
				}
				return api.OnResolveResult{Path: p}, nil
			})
			b.OnResolve(api.OnResolveOptions{Filter: `^@goesm/runtime$`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				if split {
					return api.OnResolveResult{Path: relImport(args.Importer, filepath.Join(outDir, "@goesm", "runtime.js")), External: true}, nil
				}
				return api.OnResolveResult{Path: filepath.Join(tsDir, "@goesm", "runtime", "src", "index.ts")}, nil
			})
		},
	}
}

// Build runs the whole pipeline.
func Build(opts Options) (*Result, error) {
	if opts.OutDir == "" {
		opts.OutDir = "dist"
	}
	mods, entry, err := LowerOverlay(opts.Dir, opts.Overlay, opts.Patterns)
	if err != nil {
		return nil, err
	}
	tsDir := opts.TSDir
	if tsDir == "" {
		tsDir, err = os.MkdirTemp("", "goesm-ts-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tsDir)
	}
	tsDir, _ = filepath.Abs(tsDir)
	if err := WriteTS(tsDir, mods); err != nil {
		return nil, err
	}
	outDir := opts.OutDir
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(opts.Dir, outDir)
	}

	bo := api.BuildOptions{
		Bundle:            true,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		Target:            api.ES2022, // top-level await for async package initialisers
		Sourcemap:         api.SourceMapLinked,
		SourcesContent:    api.SourcesContentInclude,
		Write:             true,
		LogLevel:          api.LogLevelSilent,
		Plugins:           []api.Plugin{resolver(tsDir, outDir, opts.Split)},
		AbsWorkingDir:     tsDir,
		MinifyWhitespace:  opts.Minify,
		MinifyIdentifiers: opts.Minify,
		MinifySyntax:      opts.Minify,
	}
	if opts.Split {
		// One ES module per Go package (dist/<import path>.js) plus the
		// runtime module (dist/@goesm/runtime.js).
		for _, m := range mods {
			bo.EntryPointsAdvanced = append(bo.EntryPointsAdvanced, api.EntryPoint{InputPath: tsPath(tsDir, m.Path), OutputPath: m.Path})
		}
		bo.EntryPointsAdvanced = append(bo.EntryPointsAdvanced, api.EntryPoint{
			InputPath:  filepath.Join(tsDir, "@goesm", "runtime", "src", "index.ts"),
			OutputPath: "@goesm/runtime",
		})
		bo.Outdir = outDir
	} else {
		bo.EntryPoints = []string{tsPath(tsDir, entry)}
		name := entry[strings.LastIndex(entry, "/")+1:]
		bo.Outfile = filepath.Join(outDir, name+".js")
	}
	res := api.Build(bo)
	if len(res.Errors) > 0 {
		lines := []string{"internal error: esbuild rejected TypeScript generated by goesm (this is a goesm bug, not an error in your Go code):"}
		for _, m := range res.Errors {
			loc := ""
			if m.Location != nil {
				loc = fmt.Sprintf("%s:%d:%d: ", m.Location.File, m.Location.Line, m.Location.Column)
			}
			lines = append(lines, fmt.Sprintf("  %s%s [esbuild]", loc, m.Text))
		}
		return nil, &DiagError{Layer: "esbuild", Lines: lines}
	}
	r := &Result{TSDir: opts.TSDir}
	for _, f := range res.OutputFiles {
		r.Outputs = append(r.Outputs, f.Path)
	}
	return r, nil
}
