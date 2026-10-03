package natives

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	goesmruntime "github.com/goesm-dev/goesm/runtime"
)

// Every replacement is one Go file excluded from goesm's own build, and every
// function it leaves without a body has an entry in the runtime's natives
// table.
func TestReplacements(t *testing.T) {
	pkgs := Packages()
	if len(pkgs) == 0 {
		t.Fatal("no replacements")
	}
	seen := map[string]bool{}
	for _, path := range pkgs {
		if seen[path] {
			t.Errorf("%s: more than one replacement file", path)
		}
		seen[path] = true
		f, ok := Replacement(path)
		if !ok {
			t.Fatalf("%s: listed but not found", path)
		}
		if !strings.HasPrefix(string(f.Src), "//go:build goesm\n") {
			t.Errorf("%s: must start with //go:build goesm", f.Name)
		}
		file, err := parser.ParseFile(token.NewFileSet(), f.Name, f.Src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := path[strings.LastIndex(path, "/")+1:]; file.Name.Name != want {
			t.Errorf("%s: package %s, want %s", f.Name, file.Name.Name, want)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body != nil {
				continue
			}
			if fd.Recv != nil {
				t.Errorf("%s: method %s without body: natives are package-level functions", f.Name, fd.Name.Name)
				continue
			}
			if name := path + "." + fd.Name.Name; !goesmruntime.HasNative(name) {
				t.Errorf("%s has no native implementation (%s) in runtime/src/natives.ts", name, goesmruntime.NativeName(name))
			}
		}
	}
	for name := range overrides {
		if !goesmruntime.HasNative(name) {
			t.Errorf("override %s has no native implementation (%s)", name, goesmruntime.NativeName(name))
		}
	}
}
