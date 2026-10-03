package test

import (
	"errors"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

func buildErr(t *testing.T, pattern string) *build.DiagError {
	t.Helper()
	_, err := build.Build(build.Options{Dir: testdata("diag"), Patterns: []string{pattern}, OutDir: t.TempDir()})
	var de *build.DiagError
	if !errors.As(err, &de) {
		t.Fatalf("expected diagnostics, got %v", err)
	}
	return de
}

// Go syntax/type errors come from the Go frontend and are reported at their
// .go positions; they never mention generated TypeScript.
func TestTypeErrorsReportedAtGoPositions(t *testing.T) {
	de := buildErr(t, "./typeerr")
	if de.Layer != "go" {
		t.Errorf("layer = %q, want go", de.Layer)
	}
	out := de.Error()
	for _, want := range []string{"typeerr.go:4:17", "[go/types]", "typeerr.go:5:9"} {
		if !strings.Contains(out, want) {
			t.Errorf("diagnostics missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".ts") {
		t.Errorf("Go errors must not point at generated TypeScript:\n%s", out)
	}
}

// Constructs goesm cannot lower yet are goesm diagnostics at .go positions,
// clearly separated from Go compiler errors.
func TestUnsupportedReportedAsLoweringDiagnostics(t *testing.T) {
	de := buildErr(t, "./unsupported")
	out := de.Error()
	if de.Layer != "goesm" || !strings.Contains(out, "unsupported.go:8:3: backward goto is not supported yet [goesm lowering]") {
		t.Errorf("unexpected diagnostics (layer %s):\n%s", de.Layer, out)
	}
}
