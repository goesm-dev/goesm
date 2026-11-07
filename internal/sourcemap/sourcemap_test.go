package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelocate checks that the sources of a module's map are written
// relative to the module's directory, in its inline comment too.
func TestRelocate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "app", "lib", "lib.go")
	b := NewBuilder()
	i := b.AddSource(src, "package lib\n")
	b.AddSource("goesm/natives/patch/regexp/regexp.go", "package regexp\n")
	b.Add(Mapping{Source: i})
	ts := "export {};\n" + b.InlineComment("lib.ts")

	data := Relocate(b.JSON("lib.ts"), filepath.Join(root, "ts", "example.com", "app"))
	var m mapJSON
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"../../../app/lib/lib.go", "goesm/natives/patch/regexp/regexp.go"}
	if strings.Join(m.Sources, " ") != strings.Join(want, " ") {
		t.Errorf("sources = %q, want %q", m.Sources, want)
	}
	if m.Mappings != "AAAA" || len(m.SourcesContent) != 2 {
		t.Errorf("the rest of the map changed: %s", data)
	}

	got := ReplaceInline(ts, data)
	enc, ok := strings.CutPrefix(got, "export {};\n"+inlinePrefix)
	if !ok || !strings.HasSuffix(enc, "\n") {
		t.Fatalf("ReplaceInline = %q", got)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(enc, "\n"))
	if err != nil || string(dec) != string(data) {
		t.Errorf("inline map = %s, want %s", dec, data)
	}
}
