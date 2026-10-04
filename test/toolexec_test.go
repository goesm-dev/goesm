package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestToolexec builds testdata/toolexec/app with a -toolexec program that
// rewrites and adds files of package main and of the standard library's
// errors, natively and with goesm (as a flag and through GOFLAGS), and
// compares the output.
func TestToolexec(t *testing.T) {
	requireNode(t)
	dir := testdata("toolexec")
	work := t.TempDir()
	goesm := filepath.Join(work, "goesm")
	rewriter := filepath.Join(work, "rewriter")
	if out, err := exec.Command("go", "build", "-o", goesm, "../cmd/goesm").CombinedOutput(); err != nil {
		t.Fatalf("building goesm: %v\n%s", err, out)
	}
	build := exec.Command("go", "build", "-o", rewriter, "./rewriter")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the rewriter: %v\n%s", err, out)
	}
	bin := filepath.Join(work, "app")
	build = exec.Command("go", "build", "-toolexec", rewriter, "-o", bin, "./app")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -toolexec: %v\n%s", err, out)
	}
	want := runProgram(t, bin)
	if want.stdout != "rewritten\n[added init]\n[rewritten] boom\n" {
		t.Fatalf("native output not rewritten:\n%s", want.stdout)
	}

	for _, tc := range []struct {
		name string
		args []string
		env  []string
	}{
		{"flag", []string{"-toolexec", rewriter}, nil},
		{"GOFLAGS", nil, []string{"GOFLAGS=-toolexec=" + rewriter}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			cmd := exec.Command(goesm, append(append([]string{"build"}, tc.args...), "-o", out, "./app")...)
			cmd.Dir = dir
			// A fresh store: compiles the go command has cached are rebuilt.
			cmd.Env = append(append(os.Environ(), "GOESM_TOOLEXEC_CACHE="+t.TempDir()), tc.env...)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("goesm build: %v\n%s", err, b)
			}
			got := runProgram(t, "node", filepath.Join(out, "app.js"))
			if got != want {
				t.Errorf("goesm: %+v\nnative Go: %+v", got, want)
			}
		})
	}
}
