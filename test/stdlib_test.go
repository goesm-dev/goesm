package test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestStdlibStatus reports how far each standard library package (with its
// dependencies) gets through lowering, and how many standard library
// functions became stubs that panic if called. It documents status; it only
// fails if a package listed as working regresses.
func TestStdlibStatus(t *testing.T) {
	working := map[string]bool{}
	for _, p := range []string{"errors", "maps", "math_bits", "slices", "sort", "strconv", "strings", "unicode", "unicode_utf8"} {
		working["probe_"+p] = true
	}
	dirs, _ := filepath.Glob(testdata("stdlib", "probe_*"))
	for _, d := range dirs {
		name := filepath.Base(d)
		l, err := build.Lower(testdata("stdlib"), []string{"./" + name})
		var de *build.DiagError
		if err == nil {
			t.Logf("%-28s lowers (%d std functions are stubs)", name, len(l.Warnings))
			continue
		}
		if !errors.As(err, &de) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		byPkg := map[string]int{}
		for _, l := range de.Lines {
			dir := filepath.Dir(strings.SplitN(l, ":", 2)[0])
			if i := strings.Index(dir, "/src/"); i >= 0 {
				dir = dir[i+5:]
			}
			byPkg[dir]++
		}
		var parts []string
		for p, n := range byPkg {
			parts = append(parts, fmt.Sprintf("%s(%d)", p, n))
		}
		sort.Strings(parts)
		t.Logf("%-28s %d diagnostics in: %s", name, len(de.Lines), strings.Join(parts, " "))
		if working[name] {
			t.Errorf("%s regressed", name)
		}
	}
	_ = os.Stderr
}
