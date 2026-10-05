// Package natives holds goesm's target-specific replacements for standard
// library packages whose Go source depends on the gc runtime's memory layout
// (runtime, internal/reflectlite, ...). Like GopherJS's natives, but kept as
// Go source: the replacement is parsed in place of the original package's
// files, so go/types still type-checks every importer against it, and the
// rest of the standard library compiles from its ordinary Go source.
//
// The set is fixed and owned by goesm. Nothing outside this directory can add
// replacements, so importing a dependency never extends the compiler.
//
// Functions declared without a body in a replacement, like the standard
// library's own assembly and linkname declarations, are implemented by the
// TypeScript runtime (runtime/src/natives.ts, see runtime.NativeName).
package natives

import (
	"embed"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed goroot
var goroot embed.FS

//go:embed patch
var patches embed.FS

// File is the replacement source of one standard library package.
type File struct {
	Name string // display path, e.g. goesm/natives/runtime/runtime.go
	Src  []byte
}

// Replacement returns the replacement of the standard library package
// importPath, or ok == false if goesm compiles it from its original source.
// A replacement is one file, so the files of the replaced package can be
// substituted in any order (go/packages parses them concurrently).
func Replacement(importPath string) (f File, ok bool) {
	dir := path.Join("goroot", importPath)
	entries, err := fs.ReadDir(goroot, dir)
	if err != nil {
		return File{}, false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, _ := goroot.ReadFile(path.Join(dir, e.Name()))
		return File{Name: path.Join("goesm/natives", importPath, e.Name()), Src: src}, true
	}
	return File{}, false
}

// Patch returns the patch of the standard library file base (a file name)
// of package importPath. A patch is Go source: its declarations replace the
// package's declarations of the same names (see Patched) and are added to
// that file, together with its imports. Patches change a few functions of a
// package that is otherwise compiled from its original source.
func Patch(importPath, base string) (f File, ok bool) {
	name := path.Join("patch", importPath, base)
	src, err := patches.ReadFile(name)
	if err != nil {
		return File{}, false
	}
	return File{Name: path.Join("goesm/natives", name), Src: src}, true
}

var patched = sync.OnceValue(func() map[string]map[string]bool {
	m := map[string]map[string]bool{}
	fs.WalkDir(patches, "patch", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		src, _ := patches.ReadFile(p)
		f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
		if err != nil {
			panic("goesm: bad patch " + p + ": " + err.Error())
		}
		pkg := strings.TrimPrefix(path.Dir(p), "patch/")
		if m[pkg] == nil {
			m[pkg] = map[string]bool{}
		}
		for _, d := range f.Decls {
			for _, n := range DeclNames(d) {
				m[pkg][n] = true
			}
		}
		return nil
	})
	return m
})

// Patched returns the names (as DeclNames reports them) of the declarations
// that the patches of package importPath replace, or nil.
func Patched(importPath string) map[string]bool { return patched()[importPath] }

// DeclNames returns the names a top-level declaration declares: "F" for a
// function, "T.M" for a method of T or *T, and the names of types, variables
// and constants. Imports declare none.
func DeclNames(d ast.Decl) []string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv != nil && len(d.Recv.List) == 1 {
			t := d.Recv.List[0].Type
			if s, ok := t.(*ast.StarExpr); ok {
				t = s.X
			}
			switch x := t.(type) {
			case *ast.IndexExpr:
				t = x.X
			case *ast.IndexListExpr:
				t = x.X
			}
			if id, ok := t.(*ast.Ident); ok {
				return []string{id.Name + "." + d.Name.Name}
			}
			return nil
		}
		return []string{d.Name.Name}
	case *ast.GenDecl:
		var names []string
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, n := range s.Names {
					names = append(names, n.Name)
				}
			}
		}
		return names
	}
	return nil
}

// Packages lists the import paths of the replaced packages.
func Packages() []string {
	var pkgs []string
	fs.WalkDir(goroot, "goroot", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") {
			pkgs = append(pkgs, strings.TrimPrefix(path.Dir(p), "goroot/"))
		}
		return err
	})
	sort.Strings(pkgs)
	return pkgs
}

// overrides are standard library functions whose Go body goesm does not
// lower: the body relies on the gc memory layout (reinterpreting memory,
// uintptr arithmetic), so the function is implemented by the runtime's
// natives table like a function without a body.
var overrides = map[string]bool{
	"internal/abi.NoEscape": true,
	"internal/abi.Escape":   true,

	// 64-bit integers are JS numbers (exact below 2^53): the bit tricks of
	// math/bits (de Bruijn multiplication, 64-bit masks) need BigInt.
	"math/bits.LeadingZeros64":  true,
	"math/bits.TrailingZeros64": true,
	"math/bits.OnesCount64":     true,
	"math/bits.RotateLeft64":    true,
	"math/bits.Reverse64":       true,
	"math/bits.ReverseBytes64":  true,
	"math/bits.Len64":           true,
	"math/bits.Add64":           true,
	"math/bits.Sub64":           true,
	"math/bits.Mul64":           true,
	"math/bits.Div64":           true,
	"math/bits.Rem64":           true,
	// The 32-bit ones widen to uint64 (BigInt); math/big's words are uint32
	// under goesm, so they are hot.
	"math/bits.Add32": true,
	"math/bits.Sub32": true,
	"math/bits.Mul32": true,
	"math/bits.Div32": true,
	"math/bits.Rem32": true,
	// The inner loops of multi-precision multiplication (see natives.ts).
	"math/big.mulAddVWW_g":                     true,
	"math/big.addMulVVWW_g":                    true,
	"crypto/internal/fips140/bigmod.addMulVVW": true,

	"slices.overlaps": true, // compares element addresses

	// iter.Pull's coroutines switch by resolving Promises (see natives.ts);
	// the Go bodies (patch/iter) only tell the blocking analysis that
	// coroswitch blocks.
	"iter.newcoro":    true,
	"iter.coroswitch": true,

	// IEEE 754 fixes their results; JS builtins are much faster than the
	// bit manipulation (on BigInt) of their Go bodies. RoundToEven's body
	// also shifts by a uint difference that wraps around.
	"math.Floor":       true,
	"math.Ceil":        true,
	"math.Trunc":       true,
	"math.Round":       true,
	"math.RoundToEven": true,
	"math.Sqrt":        true,
	"math.Abs":         true,
	"math.Signbit":     true,
	"math.Copysign":    true,
	"math.Inf":         true,

	"math.Float32bits":     true,
	"math.Float32frombits": true,
	"math.Float64bits":     true,
	"math.Float64frombits": true,

	// uint is a JS number (exact below 2^53, like int): the Go body narrows
	// its uint64 digits through uint(u).
	"internal/strconv.formatBits": true,

	"internal/strconv.float32bits":     true,
	"internal/strconv.float32frombits": true,
	"internal/strconv.float64bits":     true,
	"internal/strconv.float64frombits": true,
}

// syncFuncs are standard library functions that block only formally: a
// channel operation in them (or in function literals inside them) always
// completes at once under goesm. They are lowered as ordinary synchronous
// functions, with channel operations that panic if they would block, so
// their callers do not become async. syscall.fsCall waits on a buffered
// channel for the callback of a JavaScript fs call, which goesm's fs
// (runtime/src/natives.ts) invokes before returning.
var syncFuncs = map[string]bool{
	"syscall.fsCall": true,
}

// Sync reports whether the standard library function with the given
// types.Func.FullName is lowered as synchronous despite channel operations.
func Sync(fullName string) bool { return syncFuncs[fullName] }

// Override reports whether the standard library function with the given
// types.Func.FullName is implemented natively despite having a Go body.
func Override(fullName string) bool { return overrides[fullName] }
