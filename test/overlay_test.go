package test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// A host that embeds Go in another file (gosfc for Vue SFCs) hands goesm a
// generated file through the go command's overlay, in a directory that does
// not exist on disk. //line directives in that file make diagnostics and
// source maps point at the host file, not at the generated one.
func TestOverlayPackageWithLineDirectives(t *testing.T) {
	dir := testdata("example")
	host := filepath.Join(dir, "web", "Widget.vue")
	file := filepath.Join(dir, "web", "_gen", "widget", "widget.go")
	src := func(expr string) map[string][]byte {
		return map[string][]byte{file: []byte("package widget\n\n" +
			"import \"example.com/app/mathx\"\n\n" +
			"func Setup() int {\n" +
			"//line " + host + ":12:1\n" +
			"\tsum := " + expr + "\n" +
			"\treturn sum\n" +
			"}\n")}
	}

	mods, _, err := build.LowerOverlay(dir, src("mathx.Add(1, 2)"), []string{"./web/_gen/widget"})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range mods {
		if m.Path == "example.com/app/web/_gen/widget" {
			found = true
			if !strings.Contains(string(m.Map), "Widget.vue") {
				t.Errorf("source map does not point at the host file: %s", m.Map)
			}
		}
	}
	if !found {
		t.Fatalf("overlay package was not lowered")
	}

	_, _, err = build.LowerOverlay(dir, src(`mathx.Add(1, "x")`), []string{"./web/_gen/widget"})
	var de *build.DiagError
	if !errors.As(err, &de) {
		t.Fatalf("expected diagnostics, got %v", err)
	}
	if out := de.Error(); !strings.Contains(out, "Widget.vue:12:22") || strings.Contains(out, "widget.go") {
		t.Errorf("diagnostic not reported at the host position:\n%s", out)
	}
}
