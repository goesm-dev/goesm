package test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestPrograms runs every command under testdata/programs natively and as a
// goesm-built ES module under Node and Bun, and compares standard output,
// standard error and the exit status. For a crash (a panic nothing recovered,
// a deadlock) only the message is compared: goroutine traces differ.
// A program directory with a node-only file runs under Node only.
func TestPrograms(t *testing.T) {
	requireNode(t)
	runtimes := []string{"node"}
	if _, err := exec.LookPath("bun"); err == nil {
		runtimes = append(runtimes, "bun")
	} else if os.Getenv("GOESM_REQUIRE_TOOLS") != "" {
		t.Fatal("bun not found in PATH (GOESM_REQUIRE_TOOLS is set)")
	}
	dir := testdata("programs")
	mains, _ := filepath.Glob(filepath.Join(dir, "*", "main.go"))
	if len(mains) == 0 {
		t.Fatal("no programs found")
	}
	for _, m := range mains {
		name := filepath.Base(filepath.Dir(m))
		t.Run(name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), name)
			build := exec.Command("go", "build", "-o", bin, "./"+name)
			build.Dir = dir
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go build: %v\n%s", err, out)
			}
			want := runProgram(t, bin)
			bundle := buildPkg(t, dir, "./"+name)
			// A program with a split file also runs as one module per
			// package, where the runtime and its natives are separate
			// modules too.
			if _, err := os.Stat(filepath.Join(dir, name, "split")); err == nil {
				got := runProgram(t, "node", buildSplit(t, dir, name))
				if got != want {
					t.Errorf("split build: got %+v, native Go %+v", got, want)
				}
			}
			for _, rt := range runtimes {
				// A program with a node-only file says why it cannot run
				// under the other runtimes.
				if _, err := os.Stat(filepath.Join(dir, name, "node-only")); err == nil && rt != "node" {
					continue
				}
				got := runProgram(t, rt, bundle)
				if got.stdout != want.stdout {
					t.Errorf("%s: stdout differs:\n--- goesm\n%s--- native Go\n%s", rt, got.stdout, want.stdout)
				}
				if got.stderr != want.stderr {
					t.Errorf("%s: stderr differs:\n--- goesm\n%s--- native Go\n%s", rt, got.stderr, want.stderr)
				}
				if got.code != want.code {
					t.Errorf("%s: exit status %d, native Go %d", rt, got.code, want.code)
				}
			}
		})
	}
}

// buildSplit builds program name with -split and returns its module.
func buildSplit(t *testing.T, dir, name string) string {
	t.Helper()
	out := t.TempDir()
	res, err := build.Build(build.Options{Dir: dir, Patterns: []string{"./" + name}, OutDir: out, Split: true})
	if err != nil {
		t.Fatalf("goesm build -split %s: %v", name, err)
	}
	for _, o := range res.Outputs {
		if strings.HasSuffix(o, string(filepath.Separator)+name+".js") {
			return o
		}
	}
	t.Fatalf("no JS module for %s in %v", name, res.Outputs)
	return ""
}

type programResult struct {
	stdout, stderr string
	code           int
}

func runProgram(t *testing.T, name string, args ...string) programResult {
	t.Helper()
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	timer := time.AfterFunc(30*time.Second, func() { cmd.Process.Kill() })
	err := cmd.Run()
	timer.Stop()
	r := programResult{stdout: stdout.String(), stderr: crashMessage(stderr.String())}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		r.code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return r
}

// crashMessage cuts a crash report on standard error down to its message:
// the "panic: ..." or "fatal error: ..." lines before the goroutine traces,
// without gc's "[signal ...]" line.
func crashMessage(s string) string {
	i := strings.Index(s, "panic: ")
	if j := strings.Index(s, "fatal error: "); j >= 0 && (i < 0 || j < i) {
		i = j
	}
	if i < 0 {
		return s
	}
	head, msg := s[:i], s[i:]
	if j := strings.Index(msg, "\ngoroutine "); j >= 0 {
		msg = msg[:j]
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		if !strings.HasPrefix(l, "[signal ") {
			lines = append(lines, l)
		}
	}
	return head + strings.Join(lines, "\n") + "\n"
}
