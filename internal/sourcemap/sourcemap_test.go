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

// TestMappings checks the encoding of the mappings field: Base64 VLQ deltas
// from the previous segment, segments of one line separated by commas, a
// semicolon per generated line, and the column delta reset on a new line.
func TestMappings(t *testing.T) {
	b := NewBuilder()
	a := b.AddSource("a.go", "package a\n")
	c := b.AddSource("c.go", "package c\n")
	// Added out of order: JSON sorts by generated position.
	b.Add(Mapping{GenLine: 2, GenCol: 20, Source: a, SrcLine: 1, SrcCol: 3})
	b.Add(Mapping{GenLine: 0, GenCol: 0, Source: a, SrcLine: 0, SrcCol: 0})
	b.Add(Mapping{GenLine: 2, GenCol: 1, Source: c, SrcLine: 0, SrcCol: 0})
	b.Add(Mapping{GenLine: 0, GenCol: 4, Source: a, SrcLine: 2, SrcCol: 8})
	var m mapJSON
	if err := json.Unmarshal(b.JSON("a.ts"), &m); err != nil {
		t.Fatal(err)
	}
	// [0,0,0,0] [4,0,2,8] ; ; [1,1,-2,-8] [19,-1,1,3]
	if want := "AAAA,IAEQ;;CCFR,mBDCG"; m.Mappings != want {
		t.Errorf("mappings = %q, want %q", m.Mappings, want)
	}
	if m.Version != 3 || m.File != "a.ts" || strings.Join(m.Sources, " ") != "a.go c.go" {
		t.Errorf("map = %+v", m)
	}
}

// TestRelocateEverySource checks that every absolute source is made
// relative, also one that follows a relative source.
func TestRelocateEverySource(t *testing.T) {
	root := t.TempDir()
	b := NewBuilder()
	b.AddSource("goesm/natives/patch/regexp/regexp.go", "package regexp\n")
	b.AddSource(filepath.Join(root, "app", "main.go"), "package main\n")
	var m mapJSON
	if err := json.Unmarshal(Relocate(b.JSON("main.ts"), filepath.Join(root, "ts")), &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"goesm/natives/patch/regexp/regexp.go", "../app/main.go"}
	if strings.Join(m.Sources, " ") != strings.Join(want, " ") {
		t.Errorf("sources = %q, want %q", m.Sources, want)
	}
}

// TestReplaceInlineKeepsOther checks that ReplaceInline replaces only a map
// that ends the file, and replaces one that is the whole file.
func TestReplaceInlineKeepsOther(t *testing.T) {
	data := []byte(`{"version":3}`)
	old := NewBuilder().InlineComment("x.ts")
	for _, ts := range []string{
		"export {};\n",                  // no map
		"export {};\n" + old + "f();\n", // a map followed by code
	} {
		if got := ReplaceInline(ts, data); got != ts {
			t.Errorf("ReplaceInline(%q) = %q, want it unchanged", ts, got)
		}
	}
	if got, want := ReplaceInline(old, data), InlineComment(data); got != want {
		t.Errorf("ReplaceInline(map only) = %q, want %q", got, want)
	}
}
