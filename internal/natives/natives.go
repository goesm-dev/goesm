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
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed goroot
var goroot embed.FS

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

	"slices.overlaps": true, // compares element addresses

	"math.Float32bits":     true,
	"math.Float32frombits": true,
	"math.Float64bits":     true,
	"math.Float64frombits": true,

	"internal/strconv.float32bits":     true,
	"internal/strconv.float32frombits": true,
	"internal/strconv.float64bits":     true,
	"internal/strconv.float64frombits": true,
}

// Override reports whether the standard library function with the given
// types.Func.FullName is implemented natively despite having a Go body.
func Override(fullName string) bool { return overrides[fullName] }
