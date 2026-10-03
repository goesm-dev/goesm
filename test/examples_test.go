package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestExamples builds every example under examples/, runs its index.mjs
// (which imports the built module from ./dist) and compares the output with
// output.txt. It also runs under Bun when bun is installed.
func TestExamples(t *testing.T) {
	requireNode(t)
	root, _ := filepath.Abs(filepath.Join("..", "examples"))
	dirs, _ := filepath.Glob(filepath.Join(root, "*", "index.mjs"))
	if len(dirs) == 0 {
		t.Fatal("no examples found")
	}
	runtimes := []string{"node"}
	if _, err := exec.LookPath("bun"); err == nil {
		runtimes = append(runtimes, "bun")
	}
	for _, index := range dirs {
		dir := filepath.Dir(index)
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			if _, err := build.Build(build.Options{Dir: root, Patterns: []string{"./" + name}, OutDir: filepath.Join(tmp, "dist")}); err != nil {
				t.Fatalf("goesm build ./%s: %v", name, err)
			}
			src, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tmp, "index.mjs"), src, 0o644); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(dir, "output.txt"))
			if err != nil {
				t.Fatal(err)
			}
			for _, rt := range runtimes {
				cmd := exec.Command(rt, "index.mjs")
				cmd.Dir = tmp
				got, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s index.mjs: %v\n%s", rt, err, got)
				}
				if string(got) != string(want) {
					t.Errorf("%s output differs from output.txt:\n--- got\n%s--- want\n%s", rt, got, want)
				}
			}
		})
	}
}
