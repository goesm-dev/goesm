// Command rewriter is a -toolexec program that rewrites source the way
// compile-time instrumentation does: it replaces Go files of a compile with
// edited copies and adds files, in the build's temporary directory.
//
// It rewrites package main (a string, and an added init function) and
// package errors (errors.New, and an added file it uses), so that a build
// shows both a module's and the standard library's files rewritten. (An
// added file can only import what its package imports: the compile's
// import configuration lists no others.)
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	tool, args := os.Args[1], os.Args[2:]
	if len(args) == 1 && args[0] == "-V=full" {
		// The go command keys its cache on this line: a rewritten compile
		// must not be shared with ordinary builds.
		out, err := exec.Command(tool, args...).Output()
		if err != nil {
			fail(err)
		}
		fmt.Printf("%s rewriter\n", bytes.TrimSpace(out))
		return
	}
	if strings.TrimSuffix(filepath.Base(tool), ".exe") == "compile" {
		args = rewrite(args)
	}
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		fail(err)
	}
}

type edit struct {
	file, old, new string
}

var rules = map[string]struct {
	edits []edit
	added map[string]string
}{
	"main": {
		edits: []edit{{"main.go", `"original"`, `"rewritten"`}},
		added: map[string]string{"zz_added.go": `package main

import "fmt"

func init() { messages = append(messages, fmt.Sprint("added init")) }
`},
	},
	"errors": {
		edits: []edit{{"errors.go", "return &errorString{text}", "return &errorString{rewrittenPrefix + text}"}},
		added: map[string]string{"zz_added.go": "package errors\n\nconst rewrittenPrefix = \"[rewritten] \"\n"},
	},
}

func rewrite(args []string) []string {
	pkg, out := flag(args, "-p"), flag(args, "-o")
	r, ok := rules[pkg]
	if !ok || out == "" {
		return args
	}
	work := filepath.Dir(out)
	for _, e := range r.edits {
		for i, a := range args {
			if filepath.Base(a) != e.file {
				continue
			}
			src, err := os.ReadFile(a)
			if err != nil {
				fail(err)
			}
			if !bytes.Contains(src, []byte(e.old)) {
				fail(fmt.Errorf("%s: %q not found", a, e.old))
			}
			dst := filepath.Join(work, e.file)
			if err := os.WriteFile(dst, bytes.Replace(src, []byte(e.old), []byte(e.new), 1), 0o666); err != nil {
				fail(err)
			}
			args[i] = dst
		}
	}
	for name, src := range r.added {
		dst := filepath.Join(work, name)
		if err := os.WriteFile(dst, []byte(src), 0o666); err != nil {
			fail(err)
		}
		args = append(args, dst)
	}
	return args
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "rewriter:", err)
	os.Exit(2)
}
