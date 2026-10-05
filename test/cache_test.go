package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestMain points goesm's module cache at a directory of its own for the
// run, unless GOESMCACHE is set: the tests then share one cache, so every
// test after the first lowers through modules other programs stored, and
// no entries keyed on test binaries accumulate in the user cache.
func TestMain(m *testing.M) {
	testCacheDir = os.Getenv("GOESMCACHE")
	if testCacheDir == "" {
		dir, err := os.MkdirTemp("", "goesm-cache-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		testCacheDir = dir
		os.Setenv("GOESMCACHE", dir)
		code := m.Run()
		os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// lowerCompared lowers a package through the module cache and without it,
// and fails unless both produce the same modules and warnings.
func lowerCompared(t *testing.T, dir string, overlay map[string][]byte, pattern string) *build.Lowered {
	t.Helper()
	cached, err := build.LowerOverlay(dir, overlay, []string{pattern})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOESMCACHE", "off")
	fresh, err := build.LowerOverlay(dir, overlay, []string{pattern})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOESMCACHE", cacheDir(t))
	if fresh.Cached != 0 {
		t.Fatalf("GOESMCACHE=off took %d modules from the cache", fresh.Cached)
	}
	if len(cached.Mods) != len(fresh.Mods) {
		t.Fatalf("%d modules through the cache, %d without", len(cached.Mods), len(fresh.Mods))
	}
	for i, c := range cached.Mods {
		f := fresh.Mods[i]
		if c.Path != f.Path || c.TS != f.TS || string(c.Map) != string(f.Map) || c.Native != f.Native {
			t.Errorf("module %s from the cache differs from lowering it: %s", c.Path, firstDiff(f.TS, c.TS))
		}
	}
	if strings.Join(cached.Warnings, "\n") != strings.Join(fresh.Warnings, "\n") {
		t.Errorf("warnings through the cache:\n%s\nwithout:\n%s", strings.Join(cached.Warnings, "\n"), strings.Join(fresh.Warnings, "\n"))
	}
	return cached
}

// testCacheDir is GOESMCACHE for the run (see TestMain).
var testCacheDir string

func cacheDir(t *testing.T) string {
	if testCacheDir == "" || testCacheDir == "off" {
		t.Skip("GOESMCACHE is off")
	}
	return testCacheDir
}

// The modules taken from the cache are the modules lowering produces, for
// programs whose standard library packages other programs stored with other
// whole-program facts, and a second build takes every module from the
// cache.
func TestModuleCache(t *testing.T) {
	cacheDir(t)
	fixtures := []struct{ dir, pattern string }{
		{testdata("semantics"), "./basics"},
		{testdata("semantics"), "./goroutines"},
		{testdata("semantics"), "./generics"},
		{testdata("semantics"), "./stdlibuse"},
		{testdata("semantics"), "./jsfuncsblocking"},
		{testdata("programs"), "./iterpull"},
		{testdata("programs"), "./linkname"},
		{testdata("programs"), "./condwait"},
		{testdata("example"), "./main"},
	}
	for _, f := range fixtures {
		t.Run(filepath.Base(f.pattern), func(t *testing.T) {
			lowerCompared(t, f.dir, nil, f.pattern)
			again, err := build.Lower(f.dir, []string{f.pattern})
			if err != nil {
				t.Fatal(err)
			}
			if again.Cached != len(again.Mods) {
				t.Errorf("a second build took %d of %d modules from the cache", again.Cached, len(again.Mods))
			}
		})
	}
}

// A change in one package can change the module of a package it imports:
// a less function that blocks makes sort.Slice wait for it. The cache must
// not hand out sort's module from before the change, and must hand out each
// version again when the program changes back.
func TestModuleCacheFollowsFacts(t *testing.T) {
	cacheDir(t)
	dir := testdata("example")
	file := filepath.Join(dir, "web", "_gen", "sorted", "sorted.go")
	src := func(less string) map[string][]byte {
		return map[string][]byte{file: []byte(`package sorted

import "sort"

var ch = make(chan int, 1)

func Sorted(xs []int) []int {
	sort.Slice(xs, func(i, j int) bool {
		` + less + `
		return xs[i] < xs[j]
	})
	return xs
}
`)}
	}
	plain, blocking := src(""), src("ch <- 1; <-ch")
	sortTS := func(l *build.Lowered) string {
		for _, m := range l.Mods {
			if m.Path == "sort" {
				return m.TS
			}
		}
		t.Fatal("no module for sort")
		return ""
	}
	a := lowerCompared(t, dir, plain, "./web/_gen/sorted")
	b := lowerCompared(t, dir, blocking, "./web/_gen/sorted")
	if sortTS(a) == sortTS(b) {
		t.Fatal("a blocking less function did not change sort's module; the test no longer covers a change of facts")
	}
	if b.Cached == len(b.Mods) {
		t.Errorf("all %d modules came from the cache after the change of facts", b.Cached)
	}
	for _, overlay := range []map[string][]byte{plain, blocking} {
		l, err := build.LowerOverlay(dir, overlay, []string{"./web/_gen/sorted"})
		if err != nil {
			t.Fatal(err)
		}
		if l.Cached != len(l.Mods) {
			t.Errorf("rebuilding a version built before took %d of %d modules from the cache", l.Cached, len(l.Mods))
		}
	}
}

// The lowering reads files besides the package's Go files: the files that
// //go:embed embeds, and the host files of //line directives, whose content
// the source maps include. Changing them must change the modules.
func TestModuleCacheFollowsFiles(t *testing.T) {
	cacheDir(t)
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/files\n\ngo 1.27\n")
	write("files.go", `package files

import _ "embed"

//go:embed greeting.txt
var greeting string

func Greeting() string {
//line Host.vue:7:1
	return greeting
}
`)
	write("greeting.txt", "hello")
	write("Host.vue", "<template>one</template>")
	lowered := func() string {
		l := lowerCompared(t, dir, nil, ".")
		for _, m := range l.Mods {
			if m.Path == "example.com/files" {
				return m.TS + string(m.Map)
			}
		}
		t.Fatal("no module for example.com/files")
		return ""
	}
	first := lowered()
	write("greeting.txt", "goodbye")
	second := lowered()
	if first == second || !strings.Contains(second, "goodbye") {
		t.Errorf("the module did not follow the embedded file")
	}
	write("Host.vue", "<template>two</template>")
	third := lowered()
	if second == third || !strings.Contains(third, "two") {
		t.Errorf("the module's source map did not follow the //line host file")
	}
}
