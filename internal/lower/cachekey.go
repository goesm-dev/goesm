package lower

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// LowerPackage lowers one package of the program. LowerAll is LowerPackage
// for every package; lowering one package never depends on whether another
// was lowered, which lets internal/build reuse cached modules.
func (p *Program) LowerPackage(pkg *packages.Package, opts Options) *Module {
	if pkg.PkgPath == "unsafe" {
		return nil
	}
	return newPkgEmitter(p, pkg, pkg.PkgPath == opts.Entry).emit()
}

// Facts returns, for every package, a digest of the whole-program facts that
// lowering it reads: what the blocking, address and linkname analyses
// decided about the functions, literals, range statements and variables the
// package declares, and the answers to the per-node questions the lowering
// asks the Program (CallBlocks, RangeBlocks, WaitLock, WaitLockVal). The
// module of a package is fully determined by its own source, the source of
// the packages it depends on, these digests for itself and its
// dependencies, and the Program-wide switches; internal/build keys its cache
// on them. Positions in the digests are offsets within files, so editing one
// package does not change the digests of the others.
//
// ok is false if a fact could not be attributed to a package, in which case
// nothing may be cached.
//
// Every Program field that lowering reads must be covered here;
// TestFactsCoverProgram fails when lowering reads one that is not.
func (p *Program) Facts() (digests map[*packages.Package][32]byte, ok bool) {
	// The files of each package, including the natives patches the loader
	// merged into its files (internal/loader), whose declarations keep the
	// positions of the patch.
	files := map[*token.File]*packages.Package{}
	for _, pkg := range p.Pkgs {
		for _, f := range pkg.Syntax {
			files[p.Fset.File(f.FileStart)] = pkg
			for _, d := range f.Decls {
				files[p.Fset.File(d.Pos())] = pkg
			}
		}
	}
	facts := map[*packages.Package][]string{}
	ok = true
	// at names a position by file and offset, and finds its package.
	at := func(pos token.Pos) (*packages.Package, string) {
		tf := p.Fset.File(pos)
		if tf == nil {
			return nil, ""
		}
		pkg := files[tf]
		if pkg == nil {
			return nil, ""
		}
		return pkg, tf.Name() + ":" + strconv.Itoa(tf.Offset(pos))
	}
	add := func(kind string, key any, val string) {
		var pkg *packages.Package
		var id string
		switch k := key.(type) {
		case types.Object:
			if k.Pkg() != nil {
				pkg = p.byTypes[k.Pkg()]
			}
			if !k.Pos().IsValid() {
				id = k.String()
			} else if _, where := at(k.Pos()); where != "" {
				id = k.Name() + "@" + where
			}
		case ast.Node:
			pkg, id = at(k.Pos())
			id = fmt.Sprintf("%T@%s", k, id)
		}
		if pkg == nil || id == "" {
			ok = false
			return
		}
		facts[pkg] = append(facts[pkg], kind+" "+id+" "+val)
	}
	for k, v := range p.async {
		add("async", k, strconv.FormatBool(v))
	}
	for k, v := range p.syncOnly {
		add("syncOnly", k, strconv.FormatBool(v))
	}
	for k, v := range p.boxed {
		add("boxed", k, strconv.FormatBool(v))
	}
	for k, v := range p.linkPulls {
		add("linkPull", k, v)
	}
	for k, v := range p.linkProvides {
		add("linkProvide", k, v)
	}
	if !ok {
		return nil, false
	}

	digests = map[*packages.Package][32]byte{}
	for _, pkg := range p.Pkgs {
		fs := facts[pkg]
		sort.Strings(fs)
		h := sha256.New()
		fmt.Fprintf(h, "goesm facts\ntracksGoroutines %v\nstd %v\ndep %v\n", p.TracksGoroutines, p.std[pkg], p.Deps[pkg])
		for _, f := range fs {
			fmt.Fprintln(h, f)
		}
		// The questions the lowering asks about the package's own nodes,
		// in source order.
		info := pkg.TypesInfo
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					_, where := at(n.Pos())
					wait, locker := p.WaitLock(n)
					fmt.Fprintf(h, "call %s %v %s %v\n", where, p.CallBlocks(info, n), funcName(wait), locker)
				case *ast.RangeStmt:
					if _, ok := info.TypeOf(n.X).Underlying().(*types.Signature); ok {
						_, where := at(n.Pos())
						fmt.Fprintf(h, "range %s %v\n", where, p.RangeBlocks(info, n))
					}
				case *ast.SelectorExpr:
					if wait, locker := p.WaitLockVal(n); wait != nil {
						_, where := at(n.Pos())
						fmt.Fprintf(h, "sel %s %s %v\n", where, funcName(wait), locker)
					}
				}
				return true
			})
		}
		var d [32]byte
		h.Sum(d[:0])
		digests[pkg] = d
	}
	return digests, true
}

func funcName(fn *types.Func) string {
	if fn == nil {
		return "-"
	}
	return fn.FullName()
}

// EmbeddedFiles returns the files the //go:embed directives of pkg embed, as
// absolute paths, sorted: the lowering reads them (internal/build hashes
// them into the cache key of the package's module).
func EmbeddedFiles(fset *token.FileSet, pkg *packages.Package) ([]string, error) {
	var out []string
	for v, pats := range embedDirectives(pkg.Syntax, pkg.TypesInfo) {
		dir := filepath.Dir(fset.File(v.Pos()).Name())
		files, err := embedFiles(dir, pats)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			out = append(out, filepath.Join(dir, filepath.FromSlash(f)))
		}
	}
	sort.Strings(out)
	return out, nil
}
