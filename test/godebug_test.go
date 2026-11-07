package test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGODEBUG runs testdata/programs/hostinit with GODEBUG set: goesm reads
// the settings at the first lookup of one rather than in internal/godebug's
// init, which it leaves out with the settings when nothing looks one up.
func TestGODEBUG(t *testing.T) {
	requireNode(t)
	t.Setenv("GODEBUG", "urlmaxqueryparams=2")
	dir := testdata("programs")
	bin := filepath.Join(t.TempDir(), "hostinit")
	build := exec.Command("go", "build", "-o", bin, "./hostinit")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	want := runProgram(t, bin)
	got := runProgram(t, "node", buildPkg(t, dir, "./hostinit"))
	if got != want {
		t.Errorf("got %+v, native Go %+v", got, want)
	}
	if want.stdout[:2] != "0 " {
		t.Errorf("native Go ignored GODEBUG: %q", want.stdout)
	}
}
