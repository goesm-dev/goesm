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
// dependencies) gets through lowering. It documents status; it only fails
// if a package listed as working regresses.
func TestStdlibStatus(t *testing.T) {
	working := map[string]bool{}
	dirs, _ := filepath.Glob(testdata("stdlib", "probe_*"))
	for _, d := range dirs {
		name := filepath.Base(d)
		_, _, err := build.Lower(testdata("stdlib"), []string{"./" + name})
		var de *build.DiagError
		if err == nil {
			t.Logf("%-28s lowers cleanly", name)
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
