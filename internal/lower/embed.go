package lower

import (
	"fmt"
	"go/ast"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// //go:embed: a package variable of type string, []byte or embed.FS with
// //go:embed patterns is initialised with the contents of the matching files
// of its package directory, before any other package variable, like the data
// the gc linker writes into the binary.

// embedDirectives returns the //go:embed patterns of the package variables
// declared in files.
func embedDirectives(files []*ast.File, info *types.Info) map[*types.Var][]string {
	m := map[*types.Var][]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, s := range g.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 {
					continue
				}
				doc := vs.Doc
				if doc == nil && len(g.Specs) == 1 {
					doc = g.Doc
				}
				if doc == nil {
					continue
				}
				var pats []string
				for _, c := range doc.List {
					if rest, ok := strings.CutPrefix(c.Text, "//go:embed"); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
						pats = append(pats, embedPatterns(rest)...)
					}
				}
				if v, ok := info.Defs[vs.Names[0]].(*types.Var); ok && len(pats) > 0 {
					m[v] = pats
				}
			}
		}
	}
	return m
}

// embedPatterns splits the arguments of a //go:embed line: space-separated,
// with Go string literals for patterns containing spaces.
func embedPatterns(s string) []string {
	var pats []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		if s[0] == '"' || s[0] == '`' {
			q, err := strconv.QuotedPrefix(s)
			if err == nil {
				p, _ := strconv.Unquote(q)
				pats = append(pats, p)
				s = s[len(q):]
				continue
			}
		}
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			i = len(s)
		}
		pats = append(pats, s[:i])
		s = s[i:]
	}
	return pats
}

// embedFiles resolves patterns in the package directory dir to the embedded
// files, as slash-separated paths relative to dir, sorted.
func embedFiles(dir string, pats []string) ([]string, error) {
	set := map[string]bool{}
	for _, pat := range pats {
		all := false
		if p, ok := strings.CutPrefix(pat, "all:"); ok {
			pat, all = p, true
		}
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pat)))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern %s: no matching files found", pat)
		}
		for _, m := range matches {
			st, err := os.Stat(m)
			if err != nil {
				return nil, err
			}
			if !st.IsDir() {
				rel, _ := filepath.Rel(dir, m)
				set[filepath.ToSlash(rel)] = true
				continue
			}
			err = filepath.WalkDir(m, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if p != m && !all && (strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Type().IsRegular() {
					rel, _ := filepath.Rel(dir, p)
					set[filepath.ToSlash(rel)] = true
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	var files []string
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

// embedInit returns the initialiser of the package variable v embedding the
// files in dir matched by pats, or "" after reporting a diagnostic.
func (pe *pkgEmitter) embedInit(v *types.Var, dir string, pats []string) string {
	files, err := embedFiles(dir, pats)
	if err != nil {
		pe.errorf(v.Pos(), "go:embed: %v", err)
		return ""
	}
	read := func(f string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			pe.errorf(v.Pos(), "go:embed: %v", err)
			return "", false
		}
		return string(b), true
	}
	if n, ok := types.Unalias(v.Type()).(*types.Named); ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "embed" && n.Obj().Name() == "FS" {
		// The files and their parent directories ("dir/"), in the order
		// embed.FS searches them: by directory, then by name.
		names := map[string]bool{}
		for _, f := range files {
			names[f] = true
			for d := path.Dir(f); d != "."; d = path.Dir(d) {
				names[d+"/"] = true
			}
		}
		var list []string
		for n := range names {
			list = append(list, n)
		}
		sort.Slice(list, func(i, j int) bool {
			di, ei := embedSplit(list[i])
			dj, ej := embedSplit(list[j])
			return di < dj || di == dj && ei < ej
		})
		var entries []string
		for _, n := range list {
			data := ""
			if !strings.HasSuffix(n, "/") {
				var ok bool
				if data, ok = read(n); !ok {
					return ""
				}
			}
			entries = append(entries, "["+jsString(n)+", "+jsString(data)+"]")
		}
		return fmt.Sprintf("$rt.embedFS(%s, [%s])", pe.typeDesc(v.Type(), tpScope{}), strings.Join(entries, ", "))
	}
	if len(files) != 1 {
		pe.errorf(v.Pos(), "go:embed: invalid pattern syntax or multiple files for a %s variable", v.Type())
		return ""
	}
	data, ok := read(files[0])
	if !ok {
		return ""
	}
	if b, ok := v.Type().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
		return jsString(data)
	}
	return "$rt.stringToBytes(" + jsString(data) + ")"
}

// embedSplit splits an embed.FS name into its directory and element, as
// the go command sorts them.
func embedSplit(name string) (dir, elem string) {
	name = strings.TrimSuffix(name, "/")
	i := strings.LastIndexByte(name, '/')
	if i < 0 {
		return ".", name
	}
	return name[:i], name[i+1:]
}
