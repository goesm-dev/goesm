package lower

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// TestRecovered checks which panic values recovered treats as a value
// recover returned, whose methods the first panic already needed.
func TestRecovered(t *testing.T) {
	const src = `package p

func direct() { panic(recover()) }

func local() {
	p := recover()
	panic(p)
}

func captured() {
	var p any
	defer func() {
		p = recover()
	}()
	panic(p)
}

func param(v any) { panic(v) }

func zero() {
	var p any
	panic(p)
}

func reassigned() {
	p := recover()
	p = 1
	panic(p)
}

func addressed() {
	p := recover()
	set(&p)
	panic(p)
}

func set(p *any) {}

var global any

func pkgVar() {
	global = recover()
	panic(global)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	if _, err := (&types.Config{Importer: importer.Default()}).Check("p", fset, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"direct": true, "local": true, "captured": true}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name == "set" {
			continue
		}
		var arg ast.Expr
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
					arg = call.Args[0]
				}
			}
			return true
		})
		if got := recovered(info, fd.Body, arg); got != want[fd.Name.Name] {
			t.Errorf("%s: recovered = %v, want %v", fd.Name.Name, got, want[fd.Name.Name])
		}
	}
}
