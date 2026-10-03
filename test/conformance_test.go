package test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestGoConformance runs the Go distribution's own `// run` tests
// (GOROOT/test) through goesm and compares what the ES module prints in
// Node.js with the reference output native Go is held to. It is the same
// check Go's own test runner (cmd/internal/testdir) applies to gc: the
// program must exit successfully and its combined output must equal the
// .out file next to the test, or be empty when there is none.
//
// The suite is opt-in because it compiles hundreds of programs:
//
//	GOESM_CONFORMANCE=1 go test ./test -run TestGoConformance -v
//
// Other knobs:
//
//	GOESM_GOROOT_TEST         the test directory (default $(go env GOROOT)/test)
//	GOESM_CONFORMANCE_DIRS    comma-separated subdirectories (default conformanceDirs)
//	GOESM_CONFORMANCE_RUN     regexp on test names such as ken/divconst.go
//	GOESM_CONFORMANCE_NATIVE  =1 also runs each test with native `go run` and
//	                          leaves out tests whose native output disagrees
//	GOESM_CONFORMANCE_UPDATE  =1 rewrites the passing baseline
//	GOESM_CONFORMANCE_OUT     file to write per-test results (TSV) to
//
// Tests listed in test/conformance/passing.txt must keep passing; a new pass
// is reported so the baseline can be updated.
func TestGoConformance(t *testing.T) {
	if os.Getenv("GOESM_CONFORMANCE") == "" {
		t.Skip("set GOESM_CONFORMANCE=1 to run Go's own test suite through goesm")
	}
	requireNode(t)
	root := os.Getenv("GOESM_GOROOT_TEST")
	if root == "" {
		out, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			t.Fatalf("go env GOROOT: %v", err)
		}
		root = filepath.Join(strings.TrimSpace(string(out)), "test")
	}
	if _, err := os.Stat(filepath.Join(root, "ken")); err != nil {
		// Toolchains downloaded through GOTOOLCHAIN ship without GOROOT/test.
		t.Fatalf("no Go test directory at %s; set GOESM_GOROOT_TEST to the test directory of a full Go distribution", root)
	}

	dirs := conformanceDirs
	if s := os.Getenv("GOESM_CONFORMANCE_DIRS"); s != "" {
		dirs = strings.Split(s, ",")
	}
	var filter *regexp.Regexp
	if s := os.Getenv("GOESM_CONFORMANCE_RUN"); s != "" {
		filter = regexp.MustCompile(s)
	}
	cases, err := findRunTests(root, dirs, filter)
	if err != nil {
		t.Fatal(err)
	}

	work := t.TempDir()
	goesm := filepath.Join(work, "goesm")
	if out, err := exec.Command("go", "build", "-o", goesm, "../cmd/goesm").CombinedOutput(); err != nil {
		t.Fatalf("building goesm: %v\n%s", err, out)
	}
	driver := filepath.Join(work, "run.mjs")
	if err := os.WriteFile(driver, []byte(conformanceDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &conformanceRunner{
		root:   root,
		work:   work,
		goesm:  goesm,
		driver: driver,
		goMod:  "module conformance\n\ngo " + goLangVersion() + "\n",
		native: os.Getenv("GOESM_CONFORMANCE_NATIVE") != "",
	}

	start := time.Now()
	results := make([]conformanceResult, len(cases))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, c := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i] = r.run(c)
		}()
	}
	wg.Wait()

	report := summarize(results)
	t.Logf("Go conformance (%s, %d tests, %s):\n%s", root, len(results), time.Since(start).Round(time.Second), report)
	if out := os.Getenv("GOESM_CONFORMANCE_OUT"); out != "" {
		if err := writeResults(out, results); err != nil {
			t.Error(err)
		}
	}

	baseline := filepath.Join("conformance", "passing.txt")
	want := readBaseline(t, baseline)
	var newPasses []string
	for _, res := range results {
		passed := res.Status == statusPass
		switch {
		case want[res.Name] && !passed && !strings.HasPrefix(res.Status, "skip"):
			t.Errorf("%s regressed: %s: %s", res.Name, res.Status, res.Detail)
		case !want[res.Name] && passed:
			newPasses = append(newPasses, res.Name)
		}
	}
	if os.Getenv("GOESM_CONFORMANCE_UPDATE") != "" {
		if err := writeBaseline(baseline, want, results); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", baseline)
	} else if len(newPasses) > 0 {
		t.Logf("%d tests pass but are not in %s (rerun with GOESM_CONFORMANCE_UPDATE=1):\n  %s", len(newPasses), baseline, strings.Join(newPasses, "\n  "))
	}
}

// conformanceDirs are the GOROOT/test directories whose `// run` tests are
// language tests (as opposed to compiler, linker or ABI checks).
var conformanceDirs = []string{".", "ken", "chan", "interface", "typeparam", "fixedbugs"}

// conformanceDriver imports the bundle (running package initialisation and
// main) and exits like a Go program: as soon as main returns, without
// waiting for other goroutines; with status 2 on an unrecovered panic.
const conformanceDriver = `import { pathToFileURL } from "node:url";
try {
  await import(pathToFileURL(process.argv[2]).href);
} catch (e) {
  const msg = e && e.message !== undefined ? String(e.message) : String(e);
  process.stderr.write((msg.startsWith("panic: ") ? msg : "panic: " + msg) + "\n");
  process.exit(2);
}
process.exit(0);
`

const (
	statusPass     = "pass"
	statusFrontend = "fail:frontend" // go list / go/parser / go/types rejected it
	statusLower    = "fail:goesm"    // goesm lowering diagnostics ("not supported yet")
	statusBuild    = "fail:build"    // goesm crashed or esbuild rejected its output
	statusExit     = "fail:exit"     // the ES module panicked or exited non-zero
	statusOutput   = "fail:output"   // exited normally with the wrong output
	statusTimeout  = "fail:timeout"
	statusExcluded = "skip:excluded" // build constraints exclude js/wasm
	statusNative   = "skip:native"   // native go run disagrees with the .out file here
)

type conformanceCase struct {
	Name    string // path below GOROOT/test, e.g. ken/divconst.go
	Imports []string
}

type conformanceResult struct {
	conformanceCase
	Status string
	Detail string        // one line: the reason for a failure
	Run    time.Duration // time the ES module ran in Node
}

// findRunTests lists the tests whose recipe is a bare `// run`. Tests with
// arguments or go command flags (`// run -gcflags=...`) are left out, as are
// the multi-file *.dir tests (rundir).
func findRunTests(root string, dirs []string, filter *regexp.Regexp) ([]conformanceCase, error) {
	var cases []conformanceCase
	for _, d := range dirs {
		files, err := filepath.Glob(filepath.Join(root, d, "*.go"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			name := filepath.ToSlash(filepath.Join(d, filepath.Base(f)))
			if filter != nil && !filter.MatchString(name) {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			if recipe(src) != "run" {
				continue
			}
			c := conformanceCase{Name: name}
			if af, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly); err == nil {
				for _, im := range af.Imports {
					p, _ := strconv.Unquote(im.Path.Value)
					c.Imports = append(c.Imports, p)
				}
			}
			cases = append(cases, c)
		}
	}
	return cases, nil
}

// recipe returns the execution recipe of a GOROOT/test file: the first
// non-empty line that is not a build constraint, without its "//".
func recipe(src []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := sc.Text()
		if constraint.IsGoBuild(line) || constraint.IsPlusBuild(line) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "//"))
	}
	return ""
}

type conformanceRunner struct {
	root, work, goesm, driver, goMod string
	native                           bool
}

func (r *conformanceRunner) run(c conformanceCase) conformanceResult {
	res := conformanceResult{conformanceCase: c}
	fail := func(status, detail string) conformanceResult {
		res.Status, res.Detail = status, firstLine(detail)
		return res
	}
	src := filepath.Join(r.root, filepath.FromSlash(c.Name))
	want, err := os.ReadFile(strings.TrimSuffix(src, ".go") + ".out")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(statusBuild, err.Error())
	}

	// Each test becomes a one-package module, keeping its file name.
	dir := filepath.Join(r.work, strings.ReplaceAll(strings.TrimSuffix(c.Name, ".go"), "/", "_"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(statusBuild, err.Error())
	}
	data, _ := os.ReadFile(src)
	os.WriteFile(filepath.Join(dir, filepath.Base(src)), data, 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(r.goMod), 0o644)

	if r.native {
		got, ok := runCmd(dir, 30*time.Second, "go", "run", ".")
		if !ok || normalizeOutput(got) != string(want) {
			return fail(statusNative, "native go run does not reproduce the .out file")
		}
	}

	out, ok := runCmd(dir, 2*time.Minute, r.goesm, "build", "-o", "dist", "./")
	if !ok {
		return classifyBuildFailure(res, r.work, out)
	}
	bundle := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(l, ".js") {
			bundle = filepath.Join(dir, l)
		}
	}
	if bundle == "" {
		return fail(statusBuild, "no JS output")
	}

	t0 := time.Now()
	got, ok := runCmd(dir, 20*time.Second, "node", "--stack-size=8000", r.driver, bundle)
	res.Run = time.Since(t0)
	switch {
	case got == nil:
		return fail(statusTimeout, "no exit within 20s")
	case !ok:
		return fail(statusExit, lastLine(string(got)))
	case normalizeOutput(got) != string(want):
		return fail(statusOutput, firstDiff(string(want), normalizeOutput(got)))
	}
	res.Status = statusPass
	return res
}

// runCmd runs a command with combined output. It returns nil output on
// timeout.
func runCmd(dir string, timeout time.Duration, name string, args ...string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, false
	}
	if out == nil {
		out = []byte{}
	}
	return out, err == nil
}

var diagRE = regexp.MustCompile(`^(.*?\.go):\d+:\d+: (.*) \[([^\]]+)\]$`)

// classifyBuildFailure turns the diagnostics of a failed goesm build into
// a status and a one-line reason. Diagnostics inside the standard library
// are reported per package, since they come from what the test imports,
// not from the test itself.
func classifyBuildFailure(res conformanceResult, work string, out []byte) conformanceResult {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var stdPkgs []string
	seen := map[string]bool{}
	for _, l := range lines {
		m := diagRE.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		layer := m[3]
		if layer == "go list" || layer == "go/parser" || layer == "go/types" {
			if strings.Contains(m[2], "build constraints exclude") {
				res.Status, res.Detail = statusExcluded, m[2]
				return res
			}
			res.Status, res.Detail = statusFrontend, m[2]+" ["+layer+"]"
			return res
		}
		if i := strings.Index(m[1], "/src/"); i >= 0 && !strings.HasPrefix(m[1], work) {
			p := filepath.Dir(m[1][i+5:])
			if !seen[p] {
				seen[p] = true
				stdPkgs = append(stdPkgs, p)
			}
		}
	}
	for _, l := range lines {
		if strings.Contains(l, "build constraints exclude") {
			res.Status, res.Detail = statusExcluded, l
			return res
		}
	}
	if len(stdPkgs) > 0 {
		sort.Strings(stdPkgs)
		// The transitive list is long and the same for most tests; name
		// what the test itself imports.
		res.Status, res.Detail = statusLower, "stdlib not supported yet: imports "+strings.Join(res.Imports, ",")
		return res
	}
	for _, l := range lines {
		if m := diagRE.FindStringSubmatch(l); m != nil && m[3] == "goesm lowering" {
			res.Status, res.Detail = statusLower, m[2]
			return res
		}
	}
	res.Status, res.Detail = statusBuild, firstLine(string(out))
	return res
}

func normalizeOutput(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "panic: ") {
			return firstLine(l)
		}
	}
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return firstLine(s)
}

// firstDiff describes the first differing line of two outputs.
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return firstLine(fmt.Sprintf("line %d: want %q, got %q", i+1, wl, gl))
		}
	}
	return "outputs differ"
}

// summarize renders pass rates per directory, overall, for the tests
// without imports and per imported package, plus the most common failure
// reasons. The per-package rates show which parts of the standard library
// block the most programs.
func summarize(results []conformanceResult) string {
	type tally struct{ pass, run, skip int }
	byDir := map[string]*tally{}
	var all, noImports tally
	byImport := map[string]*tally{}
	statuses := map[string]int{}
	reasons := map[string]int{}
	for _, r := range results {
		d := filepath.Dir(r.Name)
		if byDir[d] == nil {
			byDir[d] = &tally{}
		}
		statuses[r.Status]++
		for _, t := range []*tally{byDir[d], &all} {
			if strings.HasPrefix(r.Status, "skip") {
				t.skip++
				continue
			}
			t.run++
			if r.Status == statusPass {
				t.pass++
			}
		}
		if !strings.HasPrefix(r.Status, "skip") {
			ts := []*tally{&noImports}
			if len(r.Imports) > 0 {
				ts = nil
				for _, p := range r.Imports {
					if byImport[p] == nil {
						byImport[p] = &tally{}
					}
					ts = append(ts, byImport[p])
				}
			}
			for _, t := range ts {
				t.run++
				if r.Status == statusPass {
					t.pass++
				}
			}
		}
		if strings.HasPrefix(r.Status, "fail") {
			reasons[r.Status+": "+reasonKey(r.Detail)]++
		}
	}
	pct := func(t tally) string {
		if t.run == 0 {
			return "-"
		}
		return fmt.Sprintf("%5.1f%% (%d/%d)", 100*float64(t.pass)/float64(t.run), t.pass, t.run)
	}
	var b strings.Builder
	var dirs []string
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		fmt.Fprintf(&b, "  %-12s %s, %d skipped\n", d+"/", pct(*byDir[d]), byDir[d].skip)
	}
	fmt.Fprintf(&b, "  %-12s %s, %d skipped\n", "total", pct(all), all.skip)
	fmt.Fprintf(&b, "  %-12s %s\n", "no imports", pct(noImports))
	b.WriteString("  by imported package (top 15 by tests):\n")
	imports := map[string]int{}
	for p, t := range byImport {
		imports[p] = t.run
	}
	for i, p := range sortedByCount(imports) {
		if i == 15 {
			break
		}
		fmt.Fprintf(&b, "    %-14s %s\n", p, pct(*byImport[p]))
	}
	b.WriteString("  by status:\n")
	for _, s := range sortedByCount(statuses) {
		fmt.Fprintf(&b, "    %4d %s\n", statuses[s], s)
	}
	b.WriteString("  top failure reasons:\n")
	for i, s := range sortedByCount(reasons) {
		if i == 30 {
			break
		}
		fmt.Fprintf(&b, "    %4d %s\n", reasons[s], s)
	}
	return b.String()
}

var (
	quotedRE = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	numberRE = regexp.MustCompile(`\b\d+\b`)
)

// reasonKey groups failure details that differ only in quoted text or
// numbers.
func reasonKey(detail string) string {
	if strings.HasPrefix(detail, "line ") { // output diffs are all different
		return "output differs"
	}
	if strings.HasPrefix(detail, "stdlib not supported") {
		return detail
	}
	return numberRE.ReplaceAllString(quotedRE.ReplaceAllString(detail, "…"), "N")
}

func sortedByCount(m map[string]int) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

func writeResults(path string, results []conformanceResult) error {
	var b strings.Builder
	b.WriteString("test\tstatus\timports\trun_ms\tdetail\n")
	for _, r := range results {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d\t%s\n", r.Name, r.Status, strings.Join(r.Imports, ","), r.Run.Milliseconds(), strings.ReplaceAll(r.Detail, "\t", " "))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readBaseline(t *testing.T, path string) map[string]bool {
	m := map[string]bool{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			m[l] = true
		}
	}
	return m
}

// writeBaseline records the passing tests. Entries for tests that were not
// part of this run (other directories, a GOESM_CONFORMANCE_RUN filter) are
// kept.
func writeBaseline(path string, old map[string]bool, results []conformanceResult) error {
	ran := map[string]bool{}
	keep := map[string]bool{}
	for _, r := range results {
		ran[r.Name] = true
		if r.Status == statusPass {
			keep[r.Name] = true
		}
	}
	for n := range old {
		if !ran[n] {
			keep[n] = true
		}
	}
	var names []string
	for n := range keep {
		names = append(names, n)
	}
	sort.Strings(names)
	header := "# GOROOT/test `// run` tests that goesm passes (see TestGoConformance).\n" +
		"# Regenerate with GOESM_CONFORMANCE=1 GOESM_CONFORMANCE_UPDATE=1 go test ./test -run TestGoConformance\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(header+strings.Join(names, "\n")+"\n"), 0o644)
}

// goLangVersion is the language version for the generated modules: the
// one of the toolchain running the tests (go1.27.0 -> 1.27).
func goLangVersion() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	if f := strings.SplitN(v, ".", 3); len(f) >= 2 {
		minor := f[1]
		if i := strings.IndexFunc(minor, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
			minor = minor[:i] // go1.28rc1
		}
		return f[0] + "." + minor
	}
	return v
}
