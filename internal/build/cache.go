package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/goesm-dev/goesm/internal/lower"
	"github.com/goesm-dev/goesm/internal/natives"
)

// The module cache keeps the TypeScript module goesm lowered for each
// package, keyed on everything the module is made from, so that a rebuild
// lowers only the packages whose module can have changed. The key of a
// package's module covers:
//
//   - the goesm executable (the lowering, the runtime and the natives are
//     part of it) and the environment variables that change the lowering;
//   - whether the package is the program's entry;
//   - for the package and every package it depends on: its import path, its
//     files (name, content, Go language version) and the digest of the
//     whole-program facts its lowering reads (lower.Program.Facts).
//
// The frontend and the whole-program analysis still run on every build,
// since the facts of a package can change when a package that imports it
// changes (a function value of a matching signature that now blocks, a
// package variable another package now assigns). Lowering the packages is
// most of a build's time, and a rebuild after editing one package lowers
// that package and the packages whose facts the edit changed.
//
// The cache lives in the goesm directory of the user cache directory, or in
// $GOESMCACHE; GOESMCACHE=off disables it. As in the go command's build
// cache, entries unused for five days are removed.
type modCache struct {
	dir string
}

// cacheVersion changes when the format of the entries or of the keys does.
const cacheVersion = "goesm module cache 1"

// lowerEnv are the environment variables that change the lowering.
var lowerEnv = []string{"GOESM_SPLIT64"}

const (
	trimInterval = 24 * time.Hour
	trimLimit    = 5 * 24 * time.Hour
	// touchInterval is how stale an entry's modification time may get
	// before a use updates it (trimming goes by it).
	touchInterval = time.Hour
)

// openCache returns the module cache, or nil if it is disabled or
// unavailable.
func openCache() *modCache {
	dir := os.Getenv("GOESMCACHE")
	switch dir {
	case "off":
		return nil
	case "":
		c, err := os.UserCacheDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(c, "goesm", "modules")
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil
	}
	return &modCache{dir: dir}
}

// cacheEntry is a cached module with the warnings lowering it produced.
type cacheEntry struct {
	Module lower.Module
	Warns  []lower.Diagnostic
}

func (c *modCache) path(key [32]byte) string {
	h := hex.EncodeToString(key[:])
	return filepath.Join(c.dir, h[:2], h)
}

func (c *modCache) get(key [32]byte) (*cacheEntry, bool) {
	p := c.path(key)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var e cacheEntry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return nil, false
	}
	if fi, err := os.Stat(p); err == nil && time.Since(fi.ModTime()) > touchInterval {
		now := time.Now()
		os.Chtimes(p, now, now)
	}
	return &e, true
}

// put writes an entry; a failure only means the next build lowers the
// package again.
func (c *modCache) put(key [32]byte, e *cacheEntry) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
		return
	}
	writeAtomic(p, buf.Bytes())
}

// writeAtomic writes a file through a temporary file and a rename, so that
// concurrent builds never read a partial entry.
func writeAtomic(p string, data []byte) {
	f, err := os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".tmp*")
	if err != nil {
		return
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		os.Remove(f.Name())
	}
}

// trim removes the entries unused for trimLimit, at most once per
// trimInterval.
func (c *modCache) trim() {
	mark := filepath.Join(c.dir, "trim.txt")
	if fi, err := os.Stat(mark); err == nil && time.Since(fi.ModTime()) < trimInterval {
		return
	}
	writeAtomic(mark, []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"))
	cutoff := time.Now().Add(-trimLimit)
	dirs, _ := os.ReadDir(c.dir)
	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 2 {
			continue
		}
		sub := filepath.Join(c.dir, d.Name())
		entries, _ := os.ReadDir(sub)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(sub, e.Name()))
			}
		}
	}
}

// executableID identifies the running goesm executable (or test binary) by
// the hash of its content.
var executableID = sync.OnceValues(func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
})

// lowerCached is lp.LowerAll with the module cache: the modules whose key is
// in the cache are taken from it, the others are lowered and stored. It
// also returns the number of modules taken from the cache.
func lowerCached(c *modCache, lp *lower.Program, std map[*packages.Package]bool, overlay map[string][]byte, opts lower.Options) (mods []*lower.Module, cached int) {
	keys, ok := moduleKeys(lp, std, overlay, opts)
	if !ok {
		return lp.LowerAll(opts), 0
	}
	defer c.trim()
	for _, pkg := range lp.Pkgs {
		key, ok := keys[pkg]
		if ok {
			if e, hit := c.get(key); hit {
				lp.Warns = append(lp.Warns, e.Warns...)
				m := e.Module
				mods = append(mods, &m)
				cached++
				continue
			}
		}
		diags, warns := len(lp.Diags), len(lp.Warns)
		m := lp.LowerPackage(pkg, opts)
		if m == nil {
			continue
		}
		mods = append(mods, m)
		if ok && len(lp.Diags) == diags {
			c.put(key, &cacheEntry{Module: *m, Warns: lp.Warns[warns:]})
		}
	}
	return mods, cached
}

// moduleKeys returns the cache key of every package's module (see
// modCache). A package missing from keys is not cached; ok is false if no
// package may be.
func moduleKeys(lp *lower.Program, std map[*packages.Package]bool, overlay map[string][]byte, opts lower.Options) (keys map[*packages.Package][32]byte, ok bool) {
	exe, err := executableID()
	if err != nil {
		return nil, false
	}
	facts, ok := lp.Facts()
	if !ok {
		return nil, false
	}
	// own is the part of the key a package contributes to its own key and
	// to its importers': import path, files and facts.
	own := map[*packages.Package][]byte{}
	for _, pkg := range lp.Pkgs {
		h := sha256.New()
		fmt.Fprintf(h, "package %s\n", pkg.PkgPath)
		if !hashFiles(h, lp.Fset, std[pkg], pkg, overlay) {
			continue
		}
		f := facts[pkg]
		h.Write(f[:])
		own[pkg] = h.Sum(nil)
	}

	// A package depends on the packages its (possibly replaced or patched)
	// files import, as in internal/loader.
	byTypes := map[*types.Package]*packages.Package{}
	for _, pkg := range lp.Pkgs {
		byTypes[pkg.Types] = pkg
	}
	keys = map[*packages.Package][32]byte{}
	for _, pkg := range lp.Pkgs {
		deps := map[*packages.Package]bool{}
		var visit func(*packages.Package)
		visit = func(p *packages.Package) {
			if deps[p] {
				return
			}
			deps[p] = true
			for _, ip := range p.Types.Imports() {
				d := byTypes[ip]
				if d == nil {
					deps[nil] = true // not part of the program
					continue
				}
				visit(d)
			}
		}
		visit(pkg)
		var closure []*packages.Package
		complete := true
		for d := range deps {
			if d == nil || own[d] == nil {
				complete = false
			}
			closure = append(closure, d)
		}
		if !complete {
			continue
		}
		sort.Slice(closure, func(i, j int) bool { return closure[i].PkgPath < closure[j].PkgPath })
		h := sha256.New()
		fmt.Fprintf(h, "%s\nexecutable %s\n", cacheVersion, exe)
		for _, e := range lowerEnv {
			fmt.Fprintf(h, "%s=%q\n", e, os.Getenv(e))
		}
		fmt.Fprintf(h, "module %s entry %v\n", pkg.PkgPath, pkg.PkgPath == opts.Entry)
		for _, d := range closure {
			h.Write(own[d])
		}
		var k [32]byte
		h.Sum(k[:0])
		keys[pkg] = k
	}
	return keys, true
}

// hashFiles writes what the lowering of pkg reads from files: the files as
// the loader parsed them (name, Go version and content; the content of the
// natives replacements and patches is part of the executable), the files
// whose content the source maps include (the files, their //line targets
// and the natives files, read from disk as the lowering does) and the files
// that //go:embed directives embed. It reports false if a file the module
// depends on cannot be read.
func hashFiles(h io.Writer, fset *token.FileSet, std bool, pkg *packages.Package, overlay map[string][]byte) bool {
	sum := func(data []byte, err error) string {
		if err != nil {
			return "-"
		}
		s := sha256.Sum256(data)
		return hex.EncodeToString(s[:])
	}
	var lines []string
	if r, ok := natives.Replacement(pkg.PkgPath); ok && std {
		// The files of a replaced package are parsed as the replacement
		// and empty files, in whichever order go/packages parses them.
		data, err := os.ReadFile(r.Name)
		lines = append(lines, fmt.Sprintf("natives replacement %q %s", r.Name, sum(data, err)))
	} else {
		for _, f := range pkg.Syntax {
			name := fset.File(f.FileStart).Name()
			disk, err := os.ReadFile(name)
			parsed := "executable"
			if data, ok := overlay[name]; ok {
				parsed = sum(data, nil)
			} else if filepath.IsAbs(name) {
				if err != nil {
					return false
				}
				parsed = sum(disk, nil)
			}
			lines = append(lines, fmt.Sprintf("file %q %s %s %s", name, pkg.TypesInfo.FileVersions[f], parsed, sum(disk, err)))
			sources := map[string]bool{}
			for _, d := range f.Decls {
				if n := fset.File(d.Pos()).Name(); n != name {
					sources[n] = true // a natives patch
				}
			}
			for _, t := range lineTargets(f, name) {
				sources[t] = true
			}
			for n := range sources {
				data, err := os.ReadFile(n)
				lines = append(lines, fmt.Sprintf("source %q %s", n, sum(data, err)))
			}
		}
		embedded, err := lower.EmbeddedFiles(fset, pkg)
		if err != nil {
			return false
		}
		for _, n := range embedded {
			data, err := os.ReadFile(n)
			if err != nil {
				return false
			}
			lines = append(lines, fmt.Sprintf("embed %q %s", n, sum(data, nil)))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(h, l)
	}
	return true
}

// lineTargets returns the file names of the //line and /*line*/ directives
// of f, resolved as go/scanner resolves them.
func lineTargets(f *ast.File, filename string) []string {
	var names []string
	for _, g := range f.Comments {
		for _, c := range g.List {
			text, ok := strings.CutPrefix(c.Text, "//line ")
			if !ok {
				if text, ok = strings.CutPrefix(c.Text, "/*line "); !ok {
					continue
				}
				text = strings.TrimSuffix(text, "*/")
			}
			// filename:line or filename:line:col
			i := strings.LastIndexByte(text, ':')
			if i < 0 || !digits(text[i+1:]) {
				continue
			}
			if j := strings.LastIndexByte(text[:i], ':'); j >= 0 && digits(text[j+1:i]) {
				i = j
			}
			name := text[:i]
			if name == "" {
				continue
			}
			name = filepath.Clean(name)
			if !filepath.IsAbs(name) {
				name = filepath.Join(filepath.Dir(filename), name)
			}
			names = append(names, name)
		}
	}
	return names
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
