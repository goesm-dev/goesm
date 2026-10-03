// Package test holds goesm's end-to-end tests. Every test builds real Go
// packages through the full pipeline (go/packages -> lowering -> esbuild) and
// executes the resulting ES modules in Node.js. Nothing is judged by looking
// at generated TypeScript.
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
	cmd := exec.Command("node", "--test", "--enable-source-maps", "js/")
	cmd.Env = append(os.Environ(),
		"GOESM_EXAMPLE="+example,
		"GOESM_BASICS="+basics,
		"GOESM_GENERICS="+generics,
		"GOESM_GOROUTINES="+goroutines,
		"GOESM_PANICS="+panics,
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
	for _, pkg := range []string{"basics", "generics", "goroutines", "panics"} {
		t.Run(pkg, func(t *testing.T) {
			dir := testdata("semantics")
			pkgPath := "example.com/sem/" + pkg
			funcs := goldenFuncs(t, dir, pkgPath)
			native := nativeResults(t, dir, pkgPath, funcs)
			esm := esmResults(t, buildPkg(t, dir, "./"+pkg))
			for _, f := range funcs {
				n, e := native[f.name], esm[f.name]
				if !reflect.DeepEqual(n, e) {
					nj, _ := json.Marshal(n)
					ej, _ := json.Marshal(e)
					t.Errorf("%s.%s:\n  native Go: %s\n  goesm ESM: %s", pkg, f.name, nj, ej)
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
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("native output: %v\n%s", err, out)
	}
	return m
}

const esmDriver = `
const m = await import(process.argv[1]);
const rt = m.$runtime;
const out = {};
for (const [name, f] of Object.entries(m.$goesm.funcs)) {
  if (f.type.params.length !== 0 || f.type.results.length === 0) continue;
  let r = f.fn();
  if (f.async) r = await r;
  const rs = f.type.results;
  out[name] = rs.length === 1 ? rt.toJS(rs[0], r) : rs.map((t, i) => rt.toJS(t, r[i]));
}
console.log(JSON.stringify(out));
`

func esmResults(t *testing.T, bundle string) map[string]any {
	out := runNode(t, "--enable-source-maps", "--input-type=module", "-e", esmDriver, bundle)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("ESM output: %v\n%s", err, out)
	}
	return m
}
