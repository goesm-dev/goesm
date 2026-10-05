package test

import (
	"os"
	"regexp"
	"testing"
)

// TestAsyncStaysLocal checks that functions that cannot block are lowered
// as synchronous although the program uses iter.Pull and calls of
// interfaces and function values some of whose implementations block, or
// calls functions that block only for some arguments: an async function
// makes its callers async and each call cost a turn of the event loop.
func TestAsyncStaysLocal(t *testing.T) {
	for _, c := range []struct {
		program   string
		async     []string            // functions of the main package
		sync      []string            // functions of the main package
		syncStd   map[string][]string // package path → functions
		awaitOnly string              // a conditional await of the program
	}{
		{
			program: "iterpullcalls",
			async:   []string{"sum", "firstTwo", "closure", "pairs", "main"},
			syncStd: map[string][]string{
				"strings": {"ToUpper", "IndexFunc"},
				"fmt":     {"Sprint", "Sprintf"},
				"os":      {"Getenv"},
				"slices":  {"SortFunc"},
				"reflect": {"rtype$FieldByNameFunc"},
			},
		},
		{
			program:   "asynccalls",
			async:     []string{"fill", "apply"},
			awaitOnly: `instanceof Promise \? await`,
		},
		{
			program: "paramsync",
			async:   []string{"apply", "twice", "write", "main"},
			sync:    []string{"label", "count", "digest", "build", "apply$sync", "twice$sync", "write$sync"},
			syncStd: map[string][]string{"fmt": {"Sprintf", "Fprintf$sync"}},
		},
	} {
		t.Run(c.program, func(t *testing.T) {
			bundle := buildPkg(t, testdata("programs"), "./"+c.program)
			read := func(pkgPath string) string {
				b, err := os.ReadFile(tsEntry(t, bundle, pkgPath))
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			check := func(ts, pkg string, names []string, async bool) {
				for _, n := range names {
					re := regexp.MustCompile(`(?m)^(async )?function ` + regexp.QuoteMeta(n) + `(?:<[^>]*>)?\(`)
					m := re.FindStringSubmatch(ts)
					switch {
					case m == nil:
						t.Errorf("%s.%s not found", pkg, n)
					case (m[1] != "") != async:
						t.Errorf("%s.%s: async is %v, want %v", pkg, n, m[1] != "", async)
					}
				}
			}
			main := read("programs/" + c.program)
			check(main, "main", c.async, true)
			check(main, "main", c.sync, false)
			for pkg, names := range c.syncStd {
				check(read(pkg), pkg, names, false)
			}
			if c.awaitOnly != "" && !regexp.MustCompile(c.awaitOnly).MatchString(main) {
				t.Errorf("no conditional await in %s", c.program)
			}
		})
	}
}
