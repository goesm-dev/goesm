package loader

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestModuleImportPath derives the import paths of package directories in
// the module cache.
func TestModuleImportPath(t *testing.T) {
	cache := filepath.FromSlash("/cache/mod")
	for _, c := range []struct {
		dir, want string
		ok        bool
	}{
		{"google.golang.org/protobuf@v1.36.12/internal/encoding/json", "google.golang.org/protobuf/internal/encoding/json", true},
		{"github.com/!burnt!sushi/toml@v1.5.0", "github.com/BurntSushi/toml", true},
		{"github.com/a/b/v2@v2.0.0-20260101000000-abcdef/c", "github.com/a/b/v2/c", true},
		{"cache/download/x", "", false},
	} {
		got, ok := moduleImportPath(cache, filepath.Join(cache, filepath.FromSlash(c.dir)))
		if got != c.want || ok != c.ok {
			t.Errorf("moduleImportPath(%q) = %q, %v; want %q, %v", c.dir, got, ok, c.want, c.ok)
		}
	}
	if _, ok := moduleImportPath(cache, filepath.FromSlash("/elsewhere/x@v1")); ok {
		t.Error("a directory outside the module cache has a module import path")
	}
	if _, ok := moduleImportPath("", filepath.FromSlash("/cache/mod/x@v1")); ok {
		t.Error("an empty module cache has module import paths")
	}
}

// TestModulePatch applies the protobuf JSON decoder's patch to a file that
// declares the function it replaces, and leaves a version of the file that
// does not declare it as it is.
func TestModulePatch(t *testing.T) {
	const pkg, base = "google.golang.org/protobuf/internal/encoding/json", "decode_string.go"
	parse := func(src string) (*token.FileSet, *ast.File) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, base, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return fset, f
	}
	funcs := func(f *ast.File) map[string]int {
		m := map[string]int{}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				m[fd.Name.Name]++
			}
		}
		return m
	}

	fset, f := parse("package json\n\nimport \"unicode/utf8\"\n\nvar _ = utf8.RuneSelf\n\nfunc indexNeedEscapeInBytes(b []byte) int { return 0 }\n\nfunc other() {}\n")
	got, err := modulePatch(fset, f, pkg, base)
	if err != nil {
		t.Fatal(err)
	}
	if fs := funcs(got); fs["indexNeedEscapeInBytes"] != 1 || fs["_"] != 1 || fs["other"] != 1 {
		t.Errorf("patched functions = %v; want the patch's indexNeedEscapeInBytes, the original renamed to _, and other", fs)
	}

	fset, f = parse("package json\n\nfunc renamed(b []byte) int { return 0 }\n")
	got, err = modulePatch(fset, f, pkg, base)
	if err != nil {
		t.Fatal(err)
	}
	if fs := funcs(got); len(fs) != 1 || fs["renamed"] != 1 {
		t.Errorf("functions of a version without indexNeedEscapeInBytes = %v; want it unpatched", fs)
	}
}
