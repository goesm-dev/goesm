package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestOxlint runs oxlint (pinned in test/package.json; install with
// `npm ci` in test/) on the ES modules built from the fixtures, in bundle and
// split mode, and fails on any finding of its correctness rules.
//
// oxlintrc.json turns off four rules that fire on correct generated code:
//
//   - no-unused-vars: Go allows unused parameters and package-level
//     variables, and lowering adds parameters some functions do not read
//     (type dictionaries, descriptors of generic struct methods). Unused
//     locals and imports are already Go compile errors.
//   - no-constant-condition: Go constant conditions such as
//     `if bits.UintSize == 32` are kept; minification removes them.
//   - no-unused-expressions: `_ = *p` and `_ = a[i]` evaluate the operand
//     for its nil and bounds checks.
//   - unicorn/no-new-array: the runtime allocates arrays of a length with
//     new Array(n).
//
// The test is skipped when oxlint is not installed, unless
// GOESM_REQUIRE_OXLINT is set (as in CI).
func TestOxlint(t *testing.T) {
	bin, _ := filepath.Abs(filepath.Join("node_modules", ".bin", "oxlint"))
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("GOESM_REQUIRE_OXLINT") != "" {
			t.Fatalf("oxlint is not installed: run npm ci in test/")
		}
		t.Skip("oxlint is not installed (run npm ci in test/)")
	}
	out := t.TempDir()
	type target struct {
		dir, pattern string
		split        bool
	}
	targets := []target{{testdata("example"), "./main", true}}
	for _, p := range []string{"basics", "generics", "goroutines", "panics", "stdlibuse"} {
		targets = append(targets, target{testdata("semantics"), "./" + p, false})
	}
	targets = append(targets, target{testdata("semantics"), "./stdlibuse", true})
	for i, tg := range targets {
		dst := filepath.Join(out, filepath.Base(tg.pattern))
		if tg.split {
			dst += "-split"
		}
		if _, err := build.Build(build.Options{Dir: tg.dir, Patterns: []string{tg.pattern}, OutDir: dst, Split: tg.split}); err != nil {
			t.Fatalf("goesm build %s (target %d): %v", tg.pattern, i, err)
		}
	}
	cmd := exec.Command(bin, "-c", "oxlintrc.json", "--deny-warnings", out)
	res, err := cmd.CombinedOutput()
	t.Logf("%s", res)
	if err != nil {
		t.Fatalf("oxlint reported findings on generated code: %v", err)
	}
}
