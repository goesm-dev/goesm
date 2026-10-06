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
// tables list only the methods that can be called dynamically, with the
// functions only of those reachable code calls, and the runtime is imported
// only by name.
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
		// 568 KB when the method tables kept example.com/tree's Dump,
		// which only Dump calls, with fmt, and with unicode's category
		// and script tables, which regexp/syntax needs only for \p; 223
		// KB with regexp's parser and engines, which a pattern known at
		// compile time and translated to a RegExp does not need.
		{"./regex", 60_000},
		// A pattern regexp's engines match, without the tables.
		{"./regexgo", 230_000},
		// 35 KB when the runtime's printing of panic values kept every
		// Error method, NumError's with strconv.Quote and its tables.
		{"./atoi", 20_000},
		// 210 KB with regexp's parser and engines.
		{"./datere", 40_000},
		// 553 KB with encoding/json, json v2 and jsontext, which the
		// runtime's decoder does without.
		{"./jsondecode", 45_000},
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
