package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestBundleSize builds small libraries minified and checks that the bundle
// keeps only what they use: package variables with side-effect-free
// initializers (unicode's tables) and defined types with their methods are
// declared with pure expressions, which the bundler drops when unused, method
// tables list only the methods that can be called dynamically, and the
// runtime is imported only by name.
func TestBundleSize(t *testing.T) {
	for _, c := range []struct {
		pkg string
		max int // bytes
	}{
		{"./add", 12_000},    // 71 KB when entry modules re-exported the whole runtime as $runtime
		{"./upper", 70_000},  // 363 KB when unicode's tables and every type were kept
		{"./sorted", 45_000}, // 133 KB
		// 158 KB when method tables listed every method and package
		// variables kept their initializers (internal/cpu, syscall/js).
		{"./timeonly", 48_000},
	} {
		t.Run(c.pkg[2:], func(t *testing.T) {
			out := t.TempDir()
			res, err := build.Build(build.Options{Dir: testdata("bundlesize"), Patterns: []string{c.pkg}, OutDir: out, Minify: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range res.Outputs {
				if fi, err := os.Stat(o); err == nil && filepath.Ext(o) == ".js" {
					t.Logf("%s: %d bytes", c.pkg, fi.Size())
					if fi.Size() > int64(c.max) {
						t.Errorf("%s: bundle is %d bytes, more than %d", c.pkg, fi.Size(), c.max)
					}
					return
				}
			}
			t.Fatal("no JS output")
		})
	}
}
