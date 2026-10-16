package lower

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// TestABICarriesIface: an interface reaches JavaScript through an exported
// method of a handle whose type is unexported too.
func TestABICarriesIface(t *testing.T) {
	for src, want := range map[string]bool{
		"func F() int { return 0 }":                false,
		"func F() error { return nil }":            false,
		"func F() any { return nil }":              true,
		"func F() []map[string]any { return nil }": true,
		"type h struct{}\nfunc New() *h { return nil }\nfunc (*h) Get() any { return nil }": true,
		"type h struct{}\nfunc New() *h { return nil }\nfunc (*h) get() any { return nil }": false,
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", "package p\n"+src, 0)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := (&types.Config{Importer: importer.Default()}).Check("p", fset, []*ast.File{f}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := abiCarriesIface(pkg); got != want {
			t.Errorf("%q: abiCarriesIface = %v, want %v", src, got, want)
		}
	}
}
