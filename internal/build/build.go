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
	"github.com/goesm-dev/goesm/internal/toolexec"
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
	// Toolexec is a program run on the build's compile commands, as with
	// go build -toolexec (see internal/toolexec).
	Toolexec string
}

// Result describes the outputs.
type Result struct {
	Outputs  []string // written JS files
	TSDir    string
	Warnings []string // see Lowered.Warnings
}

// DiagError carries diagnostics from one pipeline layer.
type DiagError struct {
	Layer string
	Lines []string
}

func (e *DiagError) Error() string { return strings.Join(e.Lines, "\n") }

// Lowered is the result of the frontend and the semantic lowering.
type Lowered struct {
	Mods  []*lower.Module
	Entry string // import path of the root package
	// Warnings name standard library and dependency functions that goesm
	// cannot lower yet; they were replaced by stubs that panic when called.
	Warnings []string
}

// Lower runs the Go frontend and the semantic lowering.
func Lower(dir string, patterns []string) (*Lowered, error) {
	return LowerOverlay(dir, nil, patterns)
}

// LowerOverlay is Lower with an overlay (see Options.Overlay).
func LowerOverlay(dir string, overlay map[string][]byte, patterns []string) (*Lowered, error) {
	prog, err := loader.LoadOverlay(dir, overlay, patterns...)
	if err != nil {
		if le, ok := err.(*loader.Error); ok {
			var lines []string
			for _, d := range le.Diags {
				lines = append(lines, d.String())
			}
			return nil, &DiagError{Layer: "go", Lines: lines}
		}
		return nil, err
	}
	if len(prog.Roots) != 1 {
		return nil, fmt.Errorf("goesm: expected exactly one package, got %d", len(prog.Roots))
	}
	entry := prog.Roots[0].PkgPath
	lp := lower.NewProgram(prog.Fset, prog.All, prog.Std)
	lp.Deps = prog.Deps
	mods := lp.LowerAll(lower.Options{Entry: entry})
	if len(lp.Diags) > 0 {
		var lines []string
		for _, d := range lp.SortedDiags() {
			lines = append(lines, d.String())
		}
		return nil, &DiagError{Layer: "goesm", Lines: lines}
	}
	l := &Lowered{Mods: mods, Entry: entry}
	for _, d := range lp.SortedWarns() {
		l.Warnings = append(l.Warnings, d.String())
	}
	return l, nil
}

// ToolexecOverlay runs the build of patterns through the -toolexec program
// prog and returns overlay extended with the source its compiles saw. An
// empty prog returns overlay as is.
func ToolexecOverlay(dir, prog string, overlay map[string][]byte, patterns []string) (map[string][]byte, error) {
	if prog == "" {
		return overlay, nil
	}
	if len(overlay) > 0 {
		return nil, fmt.Errorf("goesm: -toolexec cannot be combined with -overlay yet")
	}
	return toolexec.Capture(toolexec.Options{
		Dir:        dir,
		Program:    prog,
		Env:        append(os.Environ(), loader.TargetEnv...),
		BuildFlags: []string{"-tags=" + strings.Join(loader.BuildTags, ",")},
		Patterns:   patterns,
	})
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
	return filepath.Join(tsDir, filepath.FromSlash(lower.ModuleFile(importPath)))
}

// WriteTS writes generated modules and the embedded runtime to dir: the
// module of Go package p at p.ts and the runtime at @goesm/runtime/*.ts
// (see lower.ModuleFile). The tree is ready for any ESM bundler; Build
// bundles it with esbuild.
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
	return fs.WalkDir(goesmruntime.Files, "src", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := goesmruntime.Files.ReadFile(path)
		if err != nil {
			return err
		}
		out := filepath.Join(dir, filepath.FromSlash(filepath.Dir(lower.RuntimeFile)), filepath.Base(path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, append([]byte(lower.NoCheck), data...), 0o644)
	})
}

// splitResolver keeps imports between modules external in split mode: an
// import of another entry point (a Go package's module or the runtime) stays
// an ES module import of its output, with the same relative specifier ending
// in .js, since the output layout mirrors the TypeScript layout. In bundle
// mode the relative .ts imports need no resolver. It is goesm's own code;
// there is no third-party plugin mechanism.
func splitResolver(entries map[string]bool) api.Plugin {
	return api.Plugin{
		Name: "goesm-split",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: `\.ts$`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				target := filepath.Join(args.ResolveDir, filepath.FromSlash(args.Path))
				if args.Kind == api.ResolveEntryPoint || !entries[target] {
					return api.OnResolveResult{}, nil // bundled into the importer
				}
				return api.OnResolveResult{Path: strings.TrimSuffix(args.Path, ".ts") + ".js", External: true}, nil
			})
		},
	}
}

// Build runs the whole pipeline.
func Build(opts Options) (*Result, error) {
	if opts.OutDir == "" {
		opts.OutDir = "dist"
	}
	overlay, err := ToolexecOverlay(opts.Dir, opts.Toolexec, opts.Overlay, opts.Patterns)
	if err != nil {
		return nil, err
	}
	l, err := LowerOverlay(opts.Dir, overlay, opts.Patterns)
	if err != nil {
		return nil, err
	}
	mods, entry := l.Mods, l.Entry
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
		AbsWorkingDir:     tsDir,
		MinifyWhitespace:  opts.Minify,
		MinifyIdentifiers: opts.Minify,
		MinifySyntax:      opts.Minify,
	}
	if opts.Split {
		// One ES module per Go package (dist/<import path>.js) plus the
		// runtime modules (dist/@goesm/runtime/index.js and natives.js).
		entries := map[string]bool{}
		add := func(file string) {
			in := filepath.Join(tsDir, filepath.FromSlash(file))
			entries[in] = true
			bo.EntryPointsAdvanced = append(bo.EntryPointsAdvanced, api.EntryPoint{InputPath: in, OutputPath: strings.TrimSuffix(file, ".ts")})
		}
		for _, m := range mods {
			add(lower.ModuleFile(m.Path))
		}
		add(lower.RuntimeFile)
		add(lower.NativesFile)
		bo.Plugins = []api.Plugin{splitResolver(entries)}
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
	r := &Result{TSDir: opts.TSDir, Warnings: l.Warnings}
	for _, f := range res.OutputFiles {
		r.Outputs = append(r.Outputs, f.Path)
	}
	return r, nil
}
