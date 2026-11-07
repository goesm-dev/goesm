package test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFetch runs testdata/fetch, whose HTTP clients go through fetch under
// goesm unless their Transport has a custom dialer, natively and with goesm,
// against the same server.
func TestFetch(t *testing.T) {
	requireNode(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello %s", r.URL.Path)
	}))
	defer srv.Close()
	for _, c := range []struct{ pkg, want string }{
		{".", "default: 200 hello /default\nnet.Dialer: 200 hello /dialer\ncustom: error: custom dialer called\n"},
		// Without a custom dialer in the program, the Transport's own
		// connections are left out.
		{"./fetchonly", "default: 200 hello /default\nnet.Dialer: 200 hello /dialer\nclone: 200 hello /clone\n"},
	} {
		dir := testdata("fetch")
		bin := filepath.Join(t.TempDir(), "fetch")
		build := exec.Command("go", "build", "-o", bin, c.pkg)
		build.Dir = dir
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
		want := runProgram(t, bin, srv.URL)
		if want.stdout != c.want {
			t.Fatalf("%s: unexpected native output: %+v", c.pkg, want)
		}
		js := buildPkg(t, dir, c.pkg)
		runtimes := []string{"node"}
		if hasBun(t) {
			runtimes = append(runtimes, "bun")
		}
		for _, rt := range runtimes {
			if got := runProgram(t, rt, js, srv.URL); got != want {
				t.Errorf("%s, %s: %+v\nnative Go: %+v", c.pkg, rt, got, want)
			}
		}
	}
}
