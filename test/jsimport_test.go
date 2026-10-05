package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJSImport runs testdata/jsimport, which calls TypeScript and JavaScript
// functions declared with //goesm:import, under Node and Bun, and compares
// its output with want.txt (native Go cannot run it).
func TestJSImport(t *testing.T) {
	requireNode(t)
	dir := testdata("jsimport")
	want, err := os.ReadFile(filepath.Join(dir, "main", "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := buildPkg(t, dir, "./main")
	runtimes := []string{"node"}
	if hasBun(t) {
		runtimes = append(runtimes, "bun")
	}
	for _, rt := range runtimes {
		got := runProgram(t, rt, bundle)
		if got.stdout != string(want) || got.code != 0 {
			t.Errorf("%s: exit status %d, stderr %q, stdout differs:\n--- goesm\n%s--- want\n%s", rt, got.code, got.stderr, got.stdout, want)
		}
	}
	if got := runProgram(t, "node", buildSplit(t, dir, "main")); got.stdout != string(want) {
		t.Errorf("split build: stdout differs:\n--- goesm\n%s--- want\n%s", got.stdout, want)
	}
}

// Mistakes in //goesm:import directives are diagnostics at the directive.
func TestJSImportDiagnostics(t *testing.T) {
	out := buildErr(t, "./jsimport").Error()
	for _, want := range []string{
		"jsimport.go:3:1: //goesm:import: ./missing.ts does not exist [goesm lowering]",
		"jsimport.go:6:1: g cannot be imported from JavaScript: parameter 1: chan int has no JavaScript counterpart [goesm lowering]",
		"jsimport.go:9:1: //goesm:import needs a quoted module specifier",
		"jsimport.go:12:1: //goesm:import must precede a function declared without a body [goesm lowering]",
		"jsimport.go:15:1: variable V cannot be imported from JavaScript: complex128 has no JavaScript counterpart [goesm lowering]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diagnostics missing %q:\n%s", want, out)
		}
	}
}
