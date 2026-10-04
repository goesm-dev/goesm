// Package toolexec runs a -toolexec program (go build -toolexec) for goesm.
//
// goesm never runs the Go compiler: it reads Go source with go/packages.
// Tools that hook into the go command with -toolexec, such as OpenTelemetry's
// compile-time instrumentation (otelc), change the source the compiler sees
// instead: they rewrite the Go files of a compile command, add files to it,
// and hand the rewritten command on to the compiler. goesm runs the same
// build for its target (GOOS=js GOARCH=wasm, go list -export) with the
// program as -toolexec, and puts a recorder between the program and the
// compiler. The recorder notes the Go files of every compile and then runs
// the compiler. The files that differ from the package's own become an
// overlay, so goesm lowers exactly the source the compiler would compile.
//
// The go command caches compiles, and a cached compile runs no tools. The
// recorder keeps what it saw in a store next to the build cache, keyed by
// the action ID of the compile, and goesm marks the compiler's version so
// that these compiles do not share cache entries with ordinary builds.
package toolexec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Environment of the goesm processes the go command starts as tools.
const (
	envProgram = "GOESM_TOOLEXEC"          // the user's -toolexec program
	envBin     = "GOESM_TOOLEXEC_BIN"      // directory of the recorder named compile
	envStore   = "GOESM_TOOLEXEC_STORE"    // where the recorder writes
	envCompile = "GOESM_TOOLEXEC_COMPILER" // the real compiler, for the recorder
)

// ShimCommand is the goesm subcommand the go command runs as -toolexec.
const ShimCommand = "__toolexec"

// record is what the recorder saw of one compile.
type record struct {
	Pkg   string            `json:"pkg"`
	Files []string          `json:"files"`
	Saved map[string]string `json:"saved,omitempty"` // file -> contents of files in the build's temporary directory
}

// Options of a capture.
type Options struct {
	Dir        string   // working directory of the build
	Program    string   // the -toolexec program, as for go build
	Env        []string // environment of the go command (target GOOS/GOARCH)
	BuildFlags []string // e.g. -tags
	Patterns   []string
}

type listedPackage struct {
	ImportPath string
	Name       string
	Dir        string
	BuildID    string
	GoFiles    []string // relative to Dir
	Error      *struct{ Err string }
}

// Capture runs the build of opts.Patterns through opts.Program and returns
// the source the compiles saw as an overlay: file contents keyed by
// absolute path, for every file a tool replaced or added.
func Capture(opts Options) (map[string][]byte, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	store, err := storeDir()
	if err != nil {
		return nil, err
	}
	bin, err := os.MkdirTemp("", "goesm-toolexec-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(bin)
	if err := os.Symlink(self, filepath.Join(bin, "compile")); err != nil {
		return nil, err
	}
	pkgs, err := list(opts, self, bin, store, false)
	if err != nil {
		return nil, err
	}
	recs, missing := records(store, pkgs)
	if missing {
		// A compile was cached without a record (the store was cleared):
		// rebuild everything once.
		if pkgs, err = list(opts, self, bin, store, true); err != nil {
			return nil, err
		}
		if recs, missing = records(store, pkgs); missing {
			return nil, errors.New("goesm -toolexec: the compiler was not run for every package")
		}
	}
	overlay := map[string][]byte{}
	for i, p := range pkgs {
		rec := recs[i]
		if rec == nil {
			continue
		}
		orig := map[string]bool{}
		for _, f := range p.GoFiles {
			orig[filepath.Join(p.Dir, f)] = true
		}
		seen := map[string]bool{}
		for _, f := range rec.Files {
			dst := filepath.Join(p.Dir, filepath.Base(f))
			seen[dst] = true
			if orig[f] {
				continue
			}
			var data []byte
			if s, ok := rec.Saved[f]; ok {
				data = []byte(s)
			} else if data, err = os.ReadFile(f); err != nil {
				return nil, fmt.Errorf("goesm -toolexec: %s: %v", p.ImportPath, err)
			}
			overlay[dst] = data
		}
		// A file the tool dropped from the compile.
		for f := range orig {
			if !seen[f] {
				overlay[f] = []byte("package " + p.Name + "\n")
			}
		}
	}
	return overlay, nil
}

func storeDir() (string, error) {
	if d := os.Getenv("GOESM_TOOLEXEC_CACHE"); d != "" {
		return d, os.MkdirAll(d, 0o777)
	}
	c, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(c, "goesm", "toolexec")
	return d, os.MkdirAll(d, 0o777)
}

// list runs go list -export through the shim and returns the packages of
// the build (dependencies first).
func list(opts Options, self, bin, store string, all bool) ([]listedPackage, error) {
	args := []string{"list", "-deps", "-export", "-json=ImportPath,Name,Dir,BuildID,GoFiles,Error",
		"-toolexec=" + quote(self) + " " + ShimCommand}
	if all {
		args = append(args, "-a")
	}
	args = append(args, opts.BuildFlags...)
	args = append(args, "--")
	args = append(args, opts.Patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = opts.Dir
	cmd.Env = append(append([]string{}, opts.Env...), envProgram+"="+opts.Program, envBin+"="+bin, envStore+"="+store)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("goesm -toolexec: go %s: %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(&stdout)
	for {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if p.Error != nil {
			return nil, fmt.Errorf("goesm -toolexec: %s: %s", p.ImportPath, p.Error.Err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// records loads the record of each package's compile; missing reports a
// compiled package without one.
func records(store string, pkgs []listedPackage) ([]*record, bool) {
	recs := make([]*record, len(pkgs))
	missing := false
	for i, p := range pkgs {
		if p.BuildID == "" || len(p.GoFiles) == 0 {
			continue // unsafe, or nothing to compile
		}
		data, err := os.ReadFile(recordPath(store, actionID(p.BuildID)))
		if err != nil {
			missing = true
			continue
		}
		var r record
		if json.Unmarshal(data, &r) != nil {
			missing = true
			continue
		}
		recs[i] = &r
	}
	return recs, missing
}

func actionID(buildID string) string {
	id, _, _ := strings.Cut(buildID, "/")
	return id
}

func recordPath(store, actionID string) string {
	h := sha256.Sum256([]byte(actionID))
	return filepath.Join(store, hex.EncodeToString(h[:16])+".json")
}

// quote quotes a word for the go command's -toolexec splitting.
func quote(s string) string {
	if !strings.ContainsAny(s, " \t'\"") {
		return s
	}
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	return `"` + s + `"`
}

// split splits a -toolexec program into words like the go command does:
// separated by spaces, with single or double quotes around words that
// contain them.
func split(s string) ([]string, error) {
	var words []string
	for s = strings.TrimLeft(s, " \t\n\r"); s != ""; s = strings.TrimLeft(s, " \t\n\r") {
		if q := s[0]; q == '"' || q == '\'' {
			end := strings.IndexByte(s[1:], q)
			if end < 0 {
				return nil, fmt.Errorf("unterminated %c string", q)
			}
			words = append(words, s[1:1+end])
			s = s[2+end:]
			continue
		}
		i := strings.IndexAny(s, " \t\n\r")
		if i < 0 {
			i = len(s)
		}
		words = append(words, s[:i])
		s = s[i:]
	}
	return words, nil
}

// FromGOFLAGS returns the -toolexec program set in GOFLAGS, which is how
// tools such as otelc are plugged into builds they do not run themselves.
func FromGOFLAGS(goflags string) string {
	words, err := split(goflags)
	if err != nil {
		return ""
	}
	prog := ""
	for i, w := range words {
		w = strings.TrimPrefix(w, "-")
		if v, ok := strings.CutPrefix(w, "-toolexec="); ok {
			prog = v
		} else if v, ok := strings.CutPrefix(w, "toolexec="); ok {
			prog = v
		} else if (w == "toolexec" || w == "-toolexec") && i+1 < len(words) {
			prog = words[i+1]
		}
	}
	return prog
}
