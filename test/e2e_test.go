// Package test holds goesm's end-to-end tests. Every test builds real Go
// packages through the full pipeline (go/packages -> lowering -> esbuild) and
// executes the resulting ES modules in Node.js, and the emitted TypeScript
// as is in Bun. Nothing is judged by looking at generated TypeScript.
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/goesm-dev/goesm/internal/build"
)

func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found in PATH")
	}
}

func testdata(parts ...string) string {
	abs, _ := filepath.Abs(filepath.Join(append([]string{"..", "testdata"}, parts...)...))
	return abs
}

// buildPkg builds one package with goesm and returns the bundle path.
func buildPkg(t *testing.T, moduleDir, pattern string) string {
	t.Helper()
	out := t.TempDir()
	res, err := build.Build(build.Options{Dir: moduleDir, Patterns: []string{pattern}, OutDir: out, TSDir: filepath.Join(out, "ts")})
	if err != nil {
		t.Fatalf("goesm build %s: %v", pattern, err)
	}
	for _, o := range res.Outputs {
		if strings.HasSuffix(o, ".js") {
			return o
		}
	}
	t.Fatalf("no JS output for %s", pattern)
	return ""
}

func runNode(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("node", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node %v: %v\n%s", args, err, stderr.String())
	}
	return out
}

// TestJS runs the hand-written node:test suites in test/js against freshly
// built bundles. These assert the exact values requested for the PoC.
func TestJS(t *testing.T) {
	requireNode(t)
	example := buildPkg(t, testdata("example"), "./main")
	basics := buildPkg(t, testdata("semantics"), "./basics")
	generics := buildPkg(t, testdata("semantics"), "./generics")
	goroutines := buildPkg(t, testdata("semantics"), "./goroutines")
	panics := buildPkg(t, testdata("semantics"), "./panics")
	jsfuncs := buildPkg(t, testdata("semantics"), "./jsfuncs")
	jsfuncsBlocking := buildPkg(t, testdata("semantics"), "./jsfuncsblocking")
	files, _ := filepath.Glob("js/*.test.mjs")
	cmd := exec.Command("node", append([]string{"--test", "--enable-source-maps"}, files...)...)
	cmd.Env = append(os.Environ(),
		"GOESM_EXAMPLE="+example,
		"GOESM_BASICS="+basics,
		"GOESM_GENERICS="+generics,
		"GOESM_GOROUTINES="+goroutines,
		"GOESM_PANICS="+panics,
		"GOESM_JSFUNCS="+jsfuncs,
		"GOESM_JSFUNCS_BLOCKING="+jsfuncsBlocking,
	)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("node --test failed: %v", err)
	}
}

// TestGolden is the semantic oracle: for every exported, parameterless
// function of a fixture package, the result computed by native Go (go run)
// must equal the result computed by the goesm-built ES module in Node.
// Both sides are rendered as encoding/json-shaped values.
func TestGolden(t *testing.T) {
	requireNode(t)
	bun := hasBun(t)
	for _, pkg := range []string{"basics", "complexnum", "conformance", "generics", "goroutines", "int64s", "panics", "stdlibuse"} {
		t.Run(pkg, func(t *testing.T) {
			dir := testdata("semantics")
			pkgPath := "example.com/sem/" + pkg
			funcs := goldenFuncs(t, dir, pkgPath)
			native := nativeResults(t, dir, pkgPath, funcs)
			bundle := buildPkg(t, dir, "./"+pkg)
			runs := []struct{ name, runtime, module string }{{"goesm ESM", "node", bundle}}
			if bun {
				// Bun runs the per-package TypeScript as emitted.
				runs = append(runs, struct{ name, runtime, module string }{"goesm TS (Bun)", "bun", tsEntry(t, bundle, pkgPath)})
			}
			for _, r := range runs {
				esm := esmResults(t, r.runtime, r.module)
				for _, f := range funcs {
					n, e := native[f.name], esm[f.name]
					if !reflect.DeepEqual(n, e) {
						nj, _ := json.Marshal(n)
						ej, _ := json.Marshal(e)
						t.Errorf("%s.%s:\n  native Go: %s\n  %s: %s", pkg, f.name, nj, r.name, ej)
					}
				}
			}
			t.Logf("%d functions compared", len(funcs))
		})
	}
}

type goldenFunc struct {
	name    string
	results int
}

func goldenFuncs(t *testing.T, dir, pkgPath string) []goldenFunc {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: dir}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load %s: %v %v", pkgPath, err, pkgs)
	}
	scope := pkgs[0].Types.Scope()
	var out []goldenFunc
	for _, name := range scope.Names() {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig := fn.Signature()
		if sig.Params().Len() == 0 && sig.Results().Len() > 0 && sig.TypeParams().Len() == 0 {
			out = append(out, goldenFunc{name, sig.Results().Len()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func nativeResults(t *testing.T, dir, pkgPath string, funcs []goldenFunc) map[string]any {
	driverDir, err := os.MkdirTemp(dir, "_golden")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(driverDir)
	var b strings.Builder
	fmt.Fprintf(&b, "package main\n\nimport (\n\t\"encoding/json\"\n\t\"os\"\n\n\tp %q\n)\n\nfunc main() {\n\tout := map[string]any{}\n", pkgPath)
	for _, f := range funcs {
		if f.results == 1 {
			fmt.Fprintf(&b, "\tout[%q] = p.%s()\n", f.name, f.name)
			continue
		}
		var vs []string
		for i := 0; i < f.results; i++ {
			vs = append(vs, fmt.Sprintf("r%d", i))
		}
		fmt.Fprintf(&b, "\t{\n\t\t%s := p.%s()\n\t\tout[%q] = []any{%s}\n\t}\n", strings.Join(vs, ", "), f.name, f.name, strings.Join(vs, ", "))
	}
	b.WriteString("\tif err := json.NewEncoder(os.Stdout).Encode(out); err != nil {\n\t\tpanic(err)\n\t}\n}\n")
	if err := os.WriteFile(filepath.Join(driverDir, "main.go"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./"+filepath.Base(driverDir))
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("native go run: %v\n%s", err, stderr.String())
	}
	return decodeJSON(t, out)
}

const esmDriver = `
const m = await import(process.argv[process.argv.length - 1]);
const rt = m.$runtime;
const out = {};
for (const [name, f] of Object.entries(m.$goesm.funcs)) {
  if (f.type.params.length !== 0 || f.type.results.length === 0) continue;
  let r = f.fn();
  if (f.async) r = await r;
  const rs = f.type.results;
  out[name] = rs.length === 1 ? rt.toJS(rs[0], r) : rs.map((t, i) => rt.toJS(t, r[i]));
}
// int64 and uint64 are BigInts: print them as exact JSON numbers.
const big = "\u0000big:";
console.log(JSON.stringify(out, (_, v) => typeof v === "bigint" ? big + v : v).replace(/"\\u0000big:(-?[0-9]+)"/g, "$1"));
`

// esmResults runs the exported functions of module under runtime (node or
// bun) and returns their results.
func esmResults(t *testing.T, runtime, module string) map[string]any {
	t.Helper()
	driver := filepath.Join(t.TempDir(), "driver.mjs")
	if err := os.WriteFile(driver, []byte(esmDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{driver, module}
	if runtime == "node" {
		args = append([]string{"--enable-source-maps"}, args...)
	}
	cmd := exec.Command(runtime, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", runtime, args, err, stderr.String())
	}
	return decodeJSON(t, out)
}

// hasBun reports whether bun is installed; GOESM_REQUIRE_TOOLS makes it
// required.
func hasBun(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("bun"); err == nil {
		return true
	}
	if os.Getenv("GOESM_REQUIRE_TOOLS") != "" {
		t.Fatal("bun not found in PATH (GOESM_REQUIRE_TOOLS is set)")
	}
	return false
}

// tsEntry returns the emitted TypeScript module of package pkgPath next to
// a bundle built by buildPkg.
func tsEntry(t *testing.T, bundle, pkgPath string) string {
	t.Helper()
	p := filepath.Join(filepath.Dir(bundle), "ts", filepath.FromSlash(pkgPath)+".ts")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("no TypeScript output for %s: %v", pkgPath, err)
	}
	return p
}

// TestKnownGaps asserts that the documented semantic gaps (ARCHITECTURE.md,
// "Native Go differences") still exist, and shows both results.
func TestKnownGaps(t *testing.T) {
	requireNode(t)
	dir := testdata("semantics")
	pkgPath := "example.com/sem/gaps"
	funcs := goldenFuncs(t, dir, pkgPath)
	native := nativeResults(t, dir, pkgPath, funcs)
	esm := esmResults(t, "node", buildPkg(t, dir, "./gaps"))
	for _, f := range funcs {
		n, e := native[f.name], esm[f.name]
		nj, _ := json.Marshal(n)
		ej, _ := json.Marshal(e)
		if reflect.DeepEqual(n, e) {
			t.Errorf("gap %s no longer differs (native %s, ESM %s): move it to a golden fixture and update ARCHITECTURE.md", f.name, nj, ej)
			continue
		}
		t.Logf("known gap %-16s native Go %s, goesm ESM %s", f.name, nj, ej)
	}
}

// decodeJSON keeps numbers as their literal text so 64-bit values are
// compared exactly rather than after float64 rounding.
func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode results: %v\n%s", err, data)
	}
	return m
}

// TestSplitKeepsPackageBoundaries checks the per-package output: each Go
// package is its own ES module and Go imports are ES module imports.
func TestSplitKeepsPackageBoundaries(t *testing.T) {
	requireNode(t)
	out := t.TempDir()
	if _, err := build.Build(build.Options{Dir: testdata("example"), Patterns: []string{"./main"}, OutDir: out, Split: true}); err != nil {
		t.Fatal(err)
	}
	mainJS := filepath.Join(out, "example.com", "app", "main.js")
	src, err := os.ReadFile(mainJS)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `from "./mathx.js"`) {
		t.Errorf("main.js does not import the mathx package module:\n%s", src)
	}
	if strings.Contains(string(src), "function Add") {
		t.Errorf("mathx code was inlined into main.js")
	}
	got := runNode(t, "--input-type=module", "-e", `const m = await import(process.argv[1]); console.log(m.Result())`, mainJS)
	if strings.TrimSpace(string(got)) != "3" {
		t.Errorf("Result() = %s, want 3", got)
	}
}

// TestSourceMapPointsAtGo checks that the final JS source map (composed by
// esbuild from goesm's TS->Go maps) references the original .go files.
func TestSourceMapPointsAtGo(t *testing.T) {
	bundle := buildPkg(t, testdata("example"), "./main")
	data, err := os.ReadFile(bundle + ".map")
	if err != nil {
		t.Fatal(err)
	}
	var sm struct {
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
	}
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	var goSources int
	for i, s := range sm.Sources {
		if strings.HasSuffix(s, ".go") {
			goSources++
			if !strings.Contains(sm.SourcesContent[i], "package ") {
				t.Errorf("sourcesContent for %s is not Go source", s)
			}
		}
		if strings.Contains(s, "/go/example.com/") && strings.HasSuffix(s, ".ts") {
			t.Errorf("final map still points at generated TypeScript: %s", s)
		}
	}
	if goSources != 2 {
		t.Errorf("want mathx.go and main.go in sources, got %v", sm.Sources)
	}
}
