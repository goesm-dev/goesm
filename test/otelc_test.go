package test

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TestOtelc instruments testdata/otelc with otelc, OpenTelemetry's
// compile-time instrumentation for Go, once for the host (otelc setup, go
// build -toolexec) and once for goesm (GOOS=js GOARCH=wasm otelc setup, goesm
// build -toolexec), and compares the spans both print with the console
// exporter: names, kinds, attributes, scopes and how they nest, and the log
// line correlated with a span. It then compares the spans both send to an
// OTLP/HTTP collector, encoded by protobuf-go.
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

	var (
		mu      sync.Mutex
		exports [][]byte // OTLP requests
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/traces" {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			exports = append(exports, b)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/x-protobuf")
			return
		}
		fmt.Fprint(w, "hello")
	}))
	defer srv.Close()
	console := []string{"OTEL_TRACES_EXPORTER=console", "OTEL_METRICS_EXPORTER=none",
		"OTEL_LOGS_EXPORTER=none", "OTEL_GO_SIMPLE_SPAN_PROCESSOR=true"}
	otlp := []string{"OTEL_TRACES_EXPORTER=otlp", "OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		"OTEL_EXPORTER_OTLP_ENDPOINT=" + srv.URL, "OTEL_METRICS_EXPORTER=none",
		"OTEL_LOGS_EXPORTER=none", "OTEL_GO_SIMPLE_SPAN_PROCESSOR=true"}
	output := func(exporter []string, name string, args ...string) string {
		t.Helper()
		mu.Lock()
		exports = nil
		mu.Unlock()
		cmd := exec.Command(name, append(args, srv.URL)...)
		cmd.Env = append(os.Environ(), exporter...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		mu.Lock()
		defer mu.Unlock()
		return normalizeTelemetry(t, string(out)) + normalizeOTLP(t, exports)
	}
	runtimes := []string{"node"}
	if _, err := exec.LookPath("bun"); err == nil {
		runtimes = append(runtimes, "bun")
	}
	for _, exporter := range [][]string{console, otlp} {
		want := output(exporter, filepath.Join(native, "app"))
		t.Logf("native telemetry (%s):\n%s", exporter[0], want)
		if !strings.Contains(want, " parent trace=") || !strings.Contains(want, "parent=id") {
			t.Fatalf("no nested spans natively:\n%s", want)
		}
		for _, rt := range runtimes {
			if got := output(exporter, rt, filepath.Join(js, "dist", "app.js")); got != want {
				t.Errorf("%s, %s: telemetry differs:\n--- goesm\n%s--- native Go\n%s", rt, exporter[0], got, want)
			}
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

// normalizeOTLP decodes OTLP/HTTP trace export requests
// (opentelemetry.proto.collector.trace.v1.ExportTraceServiceRequest) into
// the form of normalizeTelemetry: one line per span, the resource dropped.
func normalizeOTLP(t *testing.T, reqs [][]byte) string {
	t.Helper()
	ids := map[string]string{}
	id := func(b []byte) string {
		if len(b) == 0 || strings.Trim(string(b), "\x00") == "" {
			return "-"
		}
		if ids[string(b)] == "" {
			ids[string(b)] = fmt.Sprintf("id%d", len(ids)+1)
		}
		return ids[string(b)]
	}
	var b strings.Builder
	for _, req := range reqs {
		for _, rs := range pbFields(t, req, 1) { // resource_spans
			for _, ss := range pbFields(t, rs.bytes, 2) { // scope_spans
				scope := ""
				for _, f := range pbFields(t, ss.bytes, 1) {
					scope = pbString(t, f.bytes, 1)
				}
				for _, sp := range pbFields(t, ss.bytes, 2) { // spans
					var name, trace, span, parent, kind string
					status := "0"
					var attrs []string
					for _, f := range pbFields(t, sp.bytes, 0) {
						switch f.num {
						case 1:
							trace = id(f.bytes)
						case 2:
							span = string(f.bytes)
						case 4:
							parent = id(f.bytes)
						case 5:
							name = string(f.bytes)
						case 6:
							kind = fmt.Sprint(f.varint)
						case 9:
							attrs = append(attrs, pbKeyValue(t, f.bytes))
						case 15:
							for _, c := range pbFields(t, f.bytes, 3) {
								status = fmt.Sprint(c.varint)
							}
						case 3, 7, 8, 16: // trace state, times, flags
						default:
							attrs = append(attrs, fmt.Sprintf("field%d", f.num))
						}
					}
					if parent == "" {
						parent = "-"
					}
					sort.Strings(attrs)
					fmt.Fprintf(&b, "otlp %s trace=%s parent=%s kind=%s status=%s scope=%s %s\n",
						name, trace, parent, kind, status, scope, strings.Join(attrs, " "))
					id([]byte(span))
				}
			}
		}
	}
	return b.String()
}

type pbField struct {
	num    int
	varint uint64
	bytes  []byte
}

// pbFields returns the fields numbered num (or all fields, for num 0) of
// the protobuf message b.
func pbFields(t *testing.T, b []byte, num int) []pbField {
	t.Helper()
	var fs []pbField
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatalf("bad protobuf message %x", b)
		}
		b = b[n:]
		f := pbField{num: int(key >> 3)}
		switch key & 7 {
		case 0:
			f.varint, n = binary.Uvarint(b)
		case 1:
			f.varint, n = binary.LittleEndian.Uint64(b), 8
		case 2:
			l, m := binary.Uvarint(b)
			f.bytes, n = b[m:m+int(l)], m+int(l)
		case 5:
			f.varint, n = uint64(binary.LittleEndian.Uint32(b)), 4
		default:
			t.Fatalf("bad protobuf wire type in %x", b)
		}
		b = b[n:]
		if num == 0 || f.num == num {
			fs = append(fs, f)
		}
	}
	return fs
}

func pbString(t *testing.T, b []byte, num int) string {
	s := ""
	for _, f := range pbFields(t, b, num) {
		s = string(f.bytes)
	}
	return s
}

// pbKeyValue formats an opentelemetry.proto.common.v1.KeyValue.
func pbKeyValue(t *testing.T, b []byte) string {
	key := pbString(t, b, 1)
	if key == "server.port" || key == "url.full" {
		return key + "=*"
	}
	v := ""
	for _, any := range pbFields(t, b, 2) {
		for _, f := range pbFields(t, any.bytes, 0) {
			switch f.num {
			case 1, 7: // string, bytes
				v = string(f.bytes)
			case 2, 3: // bool, int
				v = fmt.Sprint(int64(f.varint))
			case 4:
				v = fmt.Sprint(math.Float64frombits(f.varint))
			default:
				v = fmt.Sprintf("%x", f.bytes)
			}
		}
	}
	return key + "=" + v
}
