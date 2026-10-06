package lower

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/tools/go/packages"
)

// covered are the Program fields and methods that lowering a package may
// read, each of which Facts covers: in the digests of the package or of its
// dependencies (async, syncOnly, boxed, linkPulls, linkProvides, DynMethod, CalledMethod, BoxOf, std, Deps,
// TracksGoroutines, usesPull and the methods reading them), as answers recorded for
// the package's nodes (CallBlocks, CallAlwaysAsync, RangeBlocks, WaitLock,
// WaitLockVal, SyncClone, CallBlocksIn; cloned through HasClone), or
// as output the module cache keeps with the module (Diags, Warns).
var covered = map[string]bool{
	"Fset": true, "Diags": true, "Warns": true, "errorf": true,
	"async": true, "IsAsync": true, "LitAsync": true, "RangeBodyAsync": true,
	"syncOnly": true, "SyncOnly": true,
	"boxed":     true,
	"linkPulls": true, "linkProvides": true,
	"std": true, "Deps": true,
	"TracksGoroutines": true, "awaitMain": true, "usesPull": true,
	"CallBlocks": true, "CallAlwaysAsync": true, "RangeBlocks": true, "WaitLock": true, "WaitLockVal": true,
	"DynMethod": true, "CalledMethod": true, "BoxOf": true,
	// What PureEmitter answers depends on the files of an imported
	// package, which are part of its importers' module keys.
	"PureEmitter":    true,
	"UnicodeClasses": true,
	"RegexpJS":       true,
	"cloned":         true, "HasClone": true, "SyncClone": true, "CallBlocksIn": true,
}

// TestFactsCoverProgram fails when the lowering reads a Program field or
// calls a Program method that Facts does not cover: internal/build would
// then reuse a cached module that the change should have invalidated.
// NewProgram and the methods of Program are the whole-program analysis,
// which may read anything; every other function is the lowering of one
// package.
func TestFactsCoverProgram(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo, Dir: "."}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("loading the package: %v %v", err, pkgs)
	}
	pkg := pkgs[0]
	program := pkg.Types.Scope().Lookup("Program").Type()
	isProgram := func(t types.Type) bool {
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		return types.Identical(t, program)
	}
	uncovered := map[string][]string{}
	for _, f := range pkg.Syntax {
		file := filepath.Base(pkg.Fset.Position(f.FileStart).Filename)
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if fd.Name.Name == "NewProgram" || fd.Recv != nil && isProgram(pkg.TypesInfo.TypeOf(fd.Recv.List[0].Type)) {
				continue // the analysis
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if s := pkg.TypesInfo.Selections[sel]; s != nil && isProgram(s.Recv()) && !covered[sel.Sel.Name] {
					pos := pkg.Fset.Position(sel.Pos())
					uncovered[sel.Sel.Name] = append(uncovered[sel.Sel.Name], fmt.Sprintf("%s:%d", file, pos.Line))
				}
				return true
			})
		}
	}
	var names []string
	for n := range uncovered {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Errorf("the lowering reads Program.%s (%v), which Facts does not cover: add it to Facts (cachekey.go) and to covered", n, uncovered[n])
	}
}
