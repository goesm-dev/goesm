package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMemOverlay loads a package whose dependency's files are replaced and
// added beneath GOMODCACHE, where the go command accepts no overlay, with
// an added file importing a package the dependency did not import.
func TestMemOverlay(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.22\n")
	write("main.go", "package main\n\nimport \"example.com/m/dep\"\n\nfunc main() { println(dep.A()) }\n")
	write("dep/dep.go", "package dep\n\nfunc A() string { return \"a\" }\n")
	overlay := map[string][]byte{
		filepath.Join(dir, "dep/dep.go"):   []byte("package dep\n\nfunc A() string { return B() }\n"),
		filepath.Join(dir, "dep/added.go"): []byte("package dep\n\nimport \"strings\"\n\nfunc B() string { return strings.ToUpper(\"b\") }\n"),
		filepath.Join(dir, "main.go"):      []byte("package main\n\nimport \"example.com/m/dep\"\n\nfunc main() { println(dep.A(), dep.B()) }\n"),
	}
	root, _ := goEnv(dir)
	// dep is "beneath GOMODCACHE"; main.go goes to the go command's overlay.
	prog, err := load(dir, root, filepath.Join(dir, "dep"), overlay, ".")
	if err != nil {
		t.Fatal(err)
	}
	var dep, main bool
	for _, p := range prog.All {
		switch p.PkgPath {
		case "example.com/m/dep":
			dep = true
			if p.Types.Scope().Lookup("B") == nil {
				t.Errorf("dep.B (added file) not loaded")
			}
			if p.Imports["strings"] == nil {
				t.Errorf("import of strings (added file) not resolved: %v", p.Imports)
			}
			if len(p.Syntax) != 2 || len(p.CompiledGoFiles) != 2 {
				t.Errorf("dep has %d files, %d compiled files; want 2", len(p.Syntax), len(p.CompiledGoFiles))
			}
		case "example.com/m":
			main = true
		}
	}
	if !dep || !main {
		t.Errorf("packages loaded: %v", prog.All)
	}
	if len(prog.Roots) != 1 || prog.Roots[0].PkgPath != "example.com/m" {
		t.Errorf("roots: %v", prog.Roots)
	}
}
