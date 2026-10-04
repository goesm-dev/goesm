package test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestOtelc instruments testdata/otelc with otelc, OpenTelemetry's
// compile-time instrumentation for Go, once for the host (otelc setup, go
// build -toolexec) and once for goesm (GOOS=js GOARCH=wasm otelc setup, goesm
// build -toolexec), and compares the spans both print with the console
// exporter: names, kinds, attributes, scopes and how they nest, and the log
// line correlated with a span.
//
// It runs when GOESM_TEST_OTELC names an otelc binary (go install
// go.opentelemetry.io/otelc/tool/cmd/otelc@<version>), and needs network
// access for the modules otelc setup adds.
func TestOtelc(t *testing.T) {
	otelc := os.Getenv("GOESM_TEST_OTELC")
	if otelc == "" {
		t.Skip("GOESM_TEST_OTELC is not set")
	}
	requireNode(t)
	work := t.TempDir()
	goesm := filepath.Join(work, "goesm")
	if out, err := exec.Command("go", "build", "-o", goesm, "../cmd/goesm").CombinedOutput(); err != nil {
		t.Fatalf("building goesm: %v\n%s", err, out)
	}
	src, err := os.ReadFile(testdata("otelc", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(dir string, env []string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}
	setup := func(name string, env ...string) string {
		dir := filepath.Join(work, name)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "main.go"), src, 0o666)
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo "+goLangVersion()+"\n"), 0o666)
		run(dir, nil, "go", "get", "go.opentelemetry.io/otel@v1.46.0", "go.opentelemetry.io/otel/sdk@v1.46.0")
		run(dir, nil, "go", "mod", "tidy")
		run(dir, env, otelc, "setup")
		return dir
	}
	native := setup("native")
	js := setup("js", "GOOS=js", "GOARCH=wasm")
	run(native, nil, "go", "build", "-toolexec", otelc+" toolexec", "-o", "app", ".")
	run(js, nil, goesm, "build", "-toolexec", otelc+" toolexec", "-o", "dist", ".")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer srv.Close()
	exporter := []string{"OTEL_TRACES_EXPORTER=console", "OTEL_METRICS_EXPORTER=none",
		"OTEL_LOGS_EXPORTER=none", "OTEL_GO_SIMPLE_SPAN_PROCESSOR=true"}
	output := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, append(args, srv.URL)...)
		cmd.Env = append(os.Environ(), exporter...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return normalizeTelemetry(t, string(out))
	}
	want := output(filepath.Join(native, "app"))
	t.Logf("native telemetry:\n%s", want)
	if !strings.Contains(want, "span parent ") || !strings.Contains(want, "parent=id") {
		t.Fatalf("no nested spans natively:\n%s", want)
	}
	runtimes := []string{"node"}
	if _, err := exec.LookPath("bun"); err == nil {
		runtimes = append(runtimes, "bun")
	}
	for _, rt := range runtimes {
		if got := output(rt, filepath.Join(js, "dist", "app.js")); got != want {
			t.Errorf("%s: telemetry differs:\n--- goesm\n%s--- native Go\n%s", rt, got, want)
		}
	}
}

var traceIDs = regexp.MustCompile(`\b[0-9a-f]{32}\b|\b[0-9a-f]{16}\b`)

// normalizeTelemetry rewrites the console exporter's spans and the program's
// output into a comparable form: IDs are numbered in order of appearance,
// times, ports and the resource (host and process) are dropped.
func normalizeTelemetry(t *testing.T, out string) string {
	t.Helper()
	ids := map[string]string{}
	id := func(s string) string {
		if strings.Trim(s, "0") == "" {
			return "-"
		}
		if ids[s] == "" {
			ids[s] = fmt.Sprintf("id%d", len(ids)+1)
		}
		return ids[s]
	}
	type value struct {
		Key   string
		Value struct {
			Type  string
			Value any
		}
	}
	var b strings.Builder
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, `{"Name"`):
			var s struct {
				Name                 string
				SpanContext, Parent  struct{ TraceID, SpanID string }
				SpanKind             int
				Attributes           []value
				Status               struct{ Code string }
				InstrumentationScope struct{ Name string }
			}
			if err := json.Unmarshal([]byte(line), &s); err != nil {
				t.Fatalf("%v: %s", err, line)
			}
			var attrs []string
			for _, a := range s.Attributes {
				v := fmt.Sprint(a.Value.Value)
				if a.Key == "server.port" || a.Key == "url.full" {
					v = "*"
				}
				attrs = append(attrs, a.Key+"="+v)
			}
			sort.Strings(attrs)
			fmt.Fprintf(&b, "span %s trace=%s parent=%s kind=%d status=%s scope=%s %s\n", s.Name,
				id(s.SpanContext.TraceID), id(s.Parent.SpanID), s.SpanKind, s.Status.Code,
				s.InstrumentationScope.Name, strings.Join(attrs, " "))
			id(s.SpanContext.SpanID)
		case strings.HasPrefix(line, `{"time"`):
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("%v: %s", err, line)
			}
			delete(m, "time")
			for k, v := range m {
				if s, ok := v.(string); ok && (k == "trace_id" || k == "span_id") {
					m[k] = id(s)
				}
			}
			j, _ := json.Marshal(m)
			fmt.Fprintf(&b, "log %s\n", j)
		default:
			fmt.Fprintf(&b, "%s\n", traceIDs.ReplaceAllStringFunc(line, id))
		}
	}
	return b.String()
}
