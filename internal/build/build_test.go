package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goesm-dev/goesm/internal/lower"
)

// TestWriteTSRelativeImports checks that WriteTS makes the specifier of a
// //goesm:import file relative, and leaves a string literal equal to the
// file's path as it is.
func TestWriteTSRelativeImports(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "src", "lib.ts")
	ts := `import { f } from "` + filepath.ToSlash(lib) + `";` + "\n" +
		`const path = "` + filepath.ToSlash(lib) + `";` + "\n"
	m := &lower.Module{Path: "example.com/app", TS: ts, Native: true, Files: []string{lib}}
	out := filepath.Join(dir, "out")
	if err := WriteTS(out, []*lower.Module{m}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "example.com", "app.ts"))
	if err != nil {
		t.Fatal(err)
	}
	want := `import { f } from "../../src/lib.ts";` + "\n" +
		`const path = "` + filepath.ToSlash(lib) + `";` + "\n"
	if string(got) != want {
		t.Errorf("WriteTS wrote\n%s\nwant\n%s", got, want)
	}
}
