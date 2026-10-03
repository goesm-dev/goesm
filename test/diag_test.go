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
	for _, want := range []string{
		"unsupported.go:8:3: backward goto is not supported yet [goesm lowering]",
		"unsupported.go:26:12: call of a function that may block in a range-over-func body is not supported yet [goesm lowering]",
	} {
		if de.Layer != "goesm" || !strings.Contains(out, want) {
			t.Errorf("diagnostics missing %q (layer %s):\n%s", want, de.Layer, out)
		}
	}
}

// A mutex held across an operation that may block builds. Where goesm
// cannot make every Lock of that mutex wait (it is reached through a pointer
// or has no name), it warns at the blocking operation.
func TestLockAcrossBlockingWarns(t *testing.T) {
	res, err := build.Build(build.Options{Dir: testdata("diag"), Patterns: []string{"./lockblock"}, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(res.Notes, "\n")
	for _, want := range []string{
		"lockblock.go:35:13: warning: mu is locked across an operation that may block; goesm's locks wait only where the mutex is locked as mu",
		"lockblock.go:57:7: warning: lockOf() is locked across an operation that may block",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("notes missing %q:\n%s", want, out)
		}
	}
	if len(res.Notes) != 2 {
		t.Errorf("want 2 notes, got:\n%s", out)
	}
}
