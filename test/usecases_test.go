package test

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The use cases goesm supports (docs/use-cases.md), as programs under
// testdata/usecases compared with native Go: command-line tools, a build
// tool, server-side rendering, an HTTP and Connect backend served with
// http.ListenAndServe under Node.js and Bun and as a fetch handler under
// Cloudflare Workers (workerd), the Connect client, and popular libraries.

// nativeBin builds a command of testdata/usecases natively.
func nativeBin(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	build := exec.Command("go", "build", "-o", bin, pkg)
	build.Dir = testdata("usecases")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func runtimesUnderTest(t *testing.T) []string {
	rts := []string{"node"}
	if hasBun(t) {
		rts = append(rts, "bun")
	}
	return rts
}

// runWith runs name with args, standard input and extra environment.
func runWith(t *testing.T, stdin string, env []string, name string, args ...string) programResult {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	timer := time.AfterFunc(60*time.Second, func() { cmd.Process.Kill() })
	err := cmd.Run()
	timer.Stop()
	r := programResult{stdout: stdout.String(), stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		r.code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return r
}

// TestUseCaseCLI runs command-line tools with arguments, standard input,
// environment variables and files: one on the standard library alone (flag,
// os, path/filepath, encoding/csv, text/template, log/slog, ...) and one on
// spf13/cobra.
func TestUseCaseCLI(t *testing.T) {
	requireNode(t)
	dir := testdata("usecases")
	cases := []struct {
		pkg   string
		runs  [][]string
		stdin string
	}{
		{"./cli", [][]string{{"-v", "-name", "x", "-n", "5", "-d", "2m", "rest1", "rest2"}, {"-n", "bad"}, {}}, "a b c\nd e\n"},
		{"./cobra", [][]string{{"greet", "-u", "-n", "2", "a", "b"}, {"--help"}, {"greet", "--help"}, {"greet"}, {"fail"}, {"nope"}, {"completion", "bash"}}, ""},
	}
	env := []string{"UC_ENV=yes"}
	for _, c := range cases {
		t.Run(filepath.Base(c.pkg), func(t *testing.T) {
			bin := nativeBin(t, c.pkg)
			js := buildPkg(t, dir, c.pkg)
			for _, args := range c.runs {
				// Usage messages name the executable.
				want := runWith(t, c.stdin, env, bin, args...)
				want.stdout = strings.ReplaceAll(want.stdout, bin, "prog")
				want.stderr = strings.ReplaceAll(want.stderr, bin, "prog")
				for _, rt := range runtimesUnderTest(t) {
					got := runWith(t, c.stdin, env, rt, append([]string{js}, args...)...)
					got.stdout = strings.ReplaceAll(got.stdout, js, "prog")
					got.stderr = strings.ReplaceAll(got.stderr, js, "prog")
					if got != want {
						t.Errorf("%s %v:\n--- goesm\n%+v\n--- native Go\n%+v", rt, args, got, want)
					}
				}
			}
		})
	}
}

// TestUseCaseBuildTool runs a static site generator (goldmark, yaml.v3,
// BurntSushi/toml, gzip, sha256) that reads a directory and writes another,
// and server-side rendering with html/template and embed.
func TestUseCaseBuildTool(t *testing.T) {
	requireNode(t)
	dir := testdata("usecases")
	t.Run("site", func(t *testing.T) {
		bin := nativeBin(t, "./site")
		js := buildPkg(t, dir, "./site")
		wantDir := t.TempDir()
		want := runWith(t, "", nil, bin, filepath.Join(dir, "site"), wantDir)
		wantManifest, _ := os.ReadFile(filepath.Join(wantDir, "manifest.json"))
		if want.code != 0 || len(wantManifest) == 0 {
			t.Fatalf("native run: %+v", want)
		}
		for _, rt := range runtimesUnderTest(t) {
			gotDir := t.TempDir()
			got := runWith(t, "", nil, rt, js, filepath.Join(dir, "site"), gotDir)
			if got != want {
				t.Errorf("%s:\n--- goesm\n%+v\n--- native Go\n%+v", rt, got, want)
			}
			for _, f := range []string{"manifest.json", "index.html", "guide.html"} {
				w, _ := os.ReadFile(filepath.Join(wantDir, f))
				g, err := os.ReadFile(filepath.Join(gotDir, f))
				if err != nil || string(g) != string(w) {
					t.Errorf("%s: %s differs (%v)", rt, f, err)
				}
			}
		}
	})
	t.Run("ssr", func(t *testing.T) {
		want := runWith(t, "", nil, nativeBin(t, "./ssr/cmd"))
		js := buildPkg(t, dir, "./ssr/cmd")
		for _, rt := range runtimesUnderTest(t) {
			if got := runWith(t, "", nil, rt, js); got != want {
				t.Errorf("%s:\n--- goesm\n%+v\n--- native Go\n%+v", rt, got, want)
			}
		}
	})
}

// TestUseCaseLibraries runs programs that exercise popular pure-Go
// libraries (testdata/usecases/libs), which also work when imported from
// TypeScript.
func TestUseCaseLibraries(t *testing.T) {
	requireNode(t)
	dir := testdata("usecases")
	libs, _ := filepath.Glob(filepath.Join(dir, "libs", "*", "main.go"))
	if len(libs) == 0 {
		t.Fatal("no library programs found")
	}
	for _, m := range libs {
		name := filepath.Base(filepath.Dir(m))
		t.Run(name, func(t *testing.T) {
			pkg := "./libs/" + name
			want := runWith(t, "", nil, nativeBin(t, pkg))
			js := buildPkg(t, dir, pkg)
			for _, rt := range runtimesUnderTest(t) {
				if got := runWith(t, "", nil, rt, js); got != want {
					t.Errorf("%s:\n--- goesm\n%+v\n--- native Go\n%+v", rt, got, want)
				}
			}
		})
	}
}

// freePort returns a TCP port on 127.0.0.1 that nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// server is a running server process.
type server struct {
	cmd    *exec.Cmd
	url    string
	stdout *strings.Builder
}

// startServer starts name args with PORT set and waits until the port
// accepts connections.
func startServer(t *testing.T, env []string, name string, args ...string) *server {
	t.Helper()
	port := freePort(t)
	cmd := exec.Command(name, args...)
	cmd.Env = append(append(os.Environ(), env...), fmt.Sprintf("PORT=%d", port))
	out := &strings.Builder{}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s %v does not listen on %s", name, args, addr)
		}
	}
	return &server{cmd: cmd, url: "http://" + addr, stdout: out}
}

// stop shuts the server down with SIGINT and returns its standard output.
func (s *server) stop(t *testing.T) string {
	t.Helper()
	s.cmd.Process.Signal(syscall.SIGINT)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server exited with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Errorf("server did not stop after SIGINT")
		s.cmd.Process.Kill()
		<-done
	}
	return s.stdout.String()
}

// TestUseCaseServer serves package edge's Handler (a REST API and a Connect
// service) with http.ListenAndServe under Node.js and Bun, and compares what
// the client gets with a native server; the client also runs as goesm
// against the native server (the Connect client over fetch).
func TestUseCaseServer(t *testing.T) {
	requireNode(t)
	dir := testdata("usecases")
	env := []string{"GREETING=hello from the environment"}
	nativeServer := nativeBin(t, "./server")
	nativeClient := nativeBin(t, "./client")
	jsServer := buildPkg(t, dir, "./server")
	jsClient := buildPkg(t, dir, "./client")

	// The API keeps state, so every client gets a server of its own.
	upstream := startServer(t, env, nativeServer)
	proxied := upstream.url + "/hello/proxied"
	ref := startServer(t, env, nativeServer)
	want := runWith(t, "", nil, nativeClient, ref.url, proxied)
	if want.code != 0 || !strings.Contains(want.stdout, "grpcweb count end: <nil>") {
		t.Fatalf("native client against the native server: %+v", want)
	}
	wantLog := ref.stop(t)
	ref = startServer(t, env, nativeServer)
	if got := runWith(t, "", nil, "node", jsClient, ref.url, proxied); got != want {
		t.Errorf("goesm client against the native server:\n--- goesm\n%s\n--- native Go\n%s", got.stdout+got.stderr, want.stdout)
	}
	for _, rt := range runtimesUnderTest(t) {
		t.Run(rt, func(t *testing.T) {
			srv := startServer(t, env, rt, jsServer)
			if got := runWith(t, "", nil, nativeClient, srv.url, proxied); got != want {
				t.Errorf("native client against the goesm server:\n--- goesm\n%s\n--- native Go\n%s", got.stdout+got.stderr, want.stdout)
			}
			if got := srv.stop(t); got != wantLog {
				t.Errorf("server output:\n--- goesm\n%s\n--- native Go\n%s", got, wantLog)
			}
		})
	}
}

// TestUseCaseEdge serves package edge's Handler as the fetch handler of a
// Cloudflare Worker, `export default { fetch: rt.fetchHandler(Handler()) }`,
// in workerd (installed by npm ci in test/), and compares what the client
// gets with a native server. GREETING is a text binding, which reaches
// os.Getenv through nodejs_compat's process.env.
func TestUseCaseEdge(t *testing.T) {
	requireNode(t)
	workerd, _ := filepath.Abs(filepath.Join("node_modules", ".bin", "workerd"))
	if _, err := os.Stat(workerd); err != nil {
		if os.Getenv("GOESM_REQUIRE_TOOLS") != "" {
			t.Fatalf("workerd is not installed: run npm ci in test/")
		}
		t.Skip("workerd is not installed (run npm ci in test/)")
	}
	dir := testdata("usecases")
	const greeting = "hello from the environment"
	nativeServer := nativeBin(t, "./server")
	upstream := startServer(t, nil, nativeServer)
	proxied := upstream.url + "/hello/proxied"
	ref := startServer(t, []string{"GREETING=" + greeting}, nativeServer)
	nativeClient := nativeBin(t, "./client")
	want := runWith(t, "", nil, nativeClient, ref.url, proxied)

	bundle := buildPkg(t, dir, "./edge")
	work := filepath.Dir(bundle)
	worker := `import { Handler, $runtime as rt } from "./` + filepath.Base(bundle) + `";
export default { fetch: rt.fetchHandler(Handler()) };
`
	if err := os.WriteFile(filepath.Join(work, "worker.js"), []byte(worker), 0o644); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	config := fmt.Sprintf(`using Workerd = import "/workerd/workerd.capnp";
const config :Workerd.Config = (
  services = [
    (name = "main", worker = .worker),
    (name = "internet", network = (allow = ["public", "private", "local"])),
  ],
  sockets = [(name = "http", address = "127.0.0.1:%d", http = (), service = "main")],
);
const worker :Workerd.Worker = (
  modules = [
    (name = "worker.js", esModule = embed "worker.js"),
    (name = %q, esModule = embed %q),
  ],
  compatibilityDate = "2025-09-01",
  compatibilityFlags = ["nodejs_compat"],
  bindings = [(name = "GREETING", text = %q)],
  globalOutbound = "internet",
);
`, port, filepath.Base(bundle), filepath.Base(bundle), greeting)
	if err := os.WriteFile(filepath.Join(work, "config.capnp"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(workerd, "serve", "config.capnp")
	cmd.Dir = work
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	logs := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			select {
			case logs <- sc.Text():
			default:
			}
		}
	}()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workerd does not listen on %s", addr)
		}
	}
	if got := runWith(t, "", nil, nativeClient, "http://"+addr, proxied); got != want {
		t.Errorf("native client against the Worker:\n--- goesm\n%s\n--- native Go\n%s", got.stdout+got.stderr, want.stdout)
		for len(logs) > 0 {
			t.Log("workerd:", <-logs)
		}
	}
}
