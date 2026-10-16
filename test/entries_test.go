package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// One program can start from several packages, as the components of an app
// do: every root package is an entry whose exports JavaScript calls, and the
// packages they share are lowered once, so their state is shared too.
func TestSeveralEntries(t *testing.T) {
	requireNode(t)
	dir := testdata("semantics")
	out := t.TempDir()
	if _, err := build.Build(build.Options{Dir: dir, Patterns: []string{"./entries/a", "./entries/b"}, OutDir: out, Split: true}); err != nil {
		t.Fatal(err)
	}
	mod := func(p string) string {
		return filepath.ToSlash(filepath.Join(out, "example.com", "sem", "entries", p+".js"))
	}
	script := `
const a = await import(process.argv[1]);
const b = await import(process.argv[2]);
const box = a.Lookup();
box.Set(7);
console.log([a.Add(2), a.Add(3), b.Count(), await b.Wait(), box.Get()].join(","));
`
	got := strings.TrimSpace(string(runNode(t, "--input-type=module", "-e", script, mod("a"), mod("b"))))
	if got != "2,5,5,5,7" {
		t.Errorf("got %s, want 2,5,5,5,7", got)
	}

	// A bundle has one entry.
	if _, err := build.Build(build.Options{Dir: dir, Patterns: []string{"./entries/a", "./entries/b"}, OutDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "one package") {
		t.Errorf("bundle of two entries: got %v", err)
	}
}
