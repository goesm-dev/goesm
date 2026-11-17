package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
)

// TestBlockingExplained checks the explanations goesm emit-ts -blocking
// writes: each function of the package that blocks, with the chain of
// calls down to the operation that waits, through interface methods,
// function values and function parameters, and past a recursion.
func TestBlockingExplained(t *testing.T) {
	dir := testdata("programs")
	l, err := build.LowerOverlay(dir, nil, []string{"./blockwhy"})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := build.WriteBlocking(out, l); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "programs", "blockwhy.blocking.json"))
	if err != nil {
		t.Fatal(err)
	}
	var funcs []build.BlockingFunc
	if err := json.Unmarshal(data, &funcs); err != nil {
		t.Fatal(err)
	}
	// The steps of a function, as file:line: what, with the files of the
	// standard library's patches cut to their base names.
	got := map[string]string{}
	for _, f := range funcs {
		name := f.Name
		if name == "" {
			name = fmt.Sprintf("literal %d:%d", f.Line, f.Column)
		}
		var steps []string
		for _, s := range f.Steps {
			what := strings.ReplaceAll(s.What, filepath.Join(dir, "blockwhy")+string(filepath.Separator), "")
			steps = append(steps, fmt.Sprintf("%s:%d: %s", filepath.Base(s.File), s.Line, what))
		}
		got[name] = strings.Join(steps, "\n")
	}
	sleep := "sleep.go:56: receives from a channel"
	want := map[string]string{
		"(main.queue).next": "main.go:16: receives from a channel",
		"main.read": "main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		"main.nap": "main.go:28: calls time.Sleep\n" + sleep,
		"main.dispatch": "main.go:33: calls a function value of type func() string, which may be main.nap\n" +
			"main.go:28: calls time.Sleep\n" + sleep,
		"main.each": "main.go:38: calls its function parameter f, which may be the function literal at main.go:45:10\n" +
			"main.go:45: calls main.read\n" +
			"main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		"main.total": "main.go:45: calls main.each\n" +
			"main.go:38: calls its function parameter f, which may be the function literal at main.go:45:10\n" +
			"main.go:45: calls main.read\n" +
			"main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		"literal 45:10": "main.go:45: calls main.read\n" +
			"main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		// odd calls even back: the chain goes on to read.
		"main.even": "main.go:52: calls main.odd\n" +
			"main.go:61: calls main.read\n" +
			"main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		"main.odd": "main.go:59: calls main.even\n" +
			"main.go:54: calls main.read\n" +
			"main.go:23: calls the method next of main.source, which may be (main.queue).next\n" +
			"main.go:16: receives from a channel",
		"main.main": "main.go:71: sends on a channel",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got[name], w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is listed as blocking:\n%s", name, got[name])
		}
	}
}
