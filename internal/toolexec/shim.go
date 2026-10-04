package toolexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Shim is goesm run by the go command as -toolexec: args are the tool and
// its arguments. It runs the user's -toolexec program on them, with the
// recorder in place of the compiler, and returns the exit status.
func Shim(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "goesm "+ShimCommand+": no tool")
		return 2
	}
	prog, err := split(os.Getenv(envProgram))
	if err != nil {
		fmt.Fprintf(os.Stderr, "goesm -toolexec: %v\n", err)
		return 2
	}
	env := os.Environ()
	tool := strings.TrimSuffix(filepath.Base(args[0]), ".exe")
	if tool == "compile" {
		env = append(env, envCompile+"="+args[0])
		args = append([]string{filepath.Join(os.Getenv(envBin), "compile")}, args[1:]...)
	}
	words := append(prog, args...)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if len(args) == 2 && args[1] == "-V=full" {
		// The go command derives every cache key from this line: mark it so
		// that compiles recorded for goesm are not shared with other builds.
		var out bytes.Buffer
		cmd.Stdout = &out
		if code := run(cmd); code != 0 {
			return code
		}
		fmt.Println(markVersion(strings.TrimSpace(out.String())))
		return 0
	}
	cmd.Stdout = os.Stdout
	return run(cmd)
}

func markVersion(line string) string {
	if strings.Contains(line, " buildID=") {
		return line + "+goesm" // a devel toolchain: only the build ID counts
	}
	return line + " goesm"
}

// IsRecorder reports whether this process is the recorder, run by a
// -toolexec program in place of the compiler.
func IsRecorder() bool {
	return strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "compile" && os.Getenv(envCompile) != ""
}

// Record is the recorder: it notes the Go files of a compile command (args)
// and runs the compiler on it.
func Record(args []string) int {
	if !(len(args) == 1 && strings.HasPrefix(args[0], "-V")) {
		if err := save(args); err != nil {
			fmt.Fprintf(os.Stderr, "goesm -toolexec: %v\n", err)
			return 2
		}
	}
	cmd := exec.Command(os.Getenv(envCompile), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return run(cmd)
}

func save(args []string) error {
	buildID, out := flagValue(args, "-buildid"), flagValue(args, "-o")
	if buildID == "" || out == "" {
		return nil
	}
	// The go command's temporary directory ($WORK): files there are gone
	// after the build, so their contents are kept.
	work := filepath.Dir(filepath.Dir(out)) + string(filepath.Separator)
	r := record{Pkg: flagValue(args, "-p"), Saved: map[string]string{}}
	for _, a := range args {
		if strings.HasPrefix(a, "-") || !strings.HasSuffix(a, ".go") {
			continue
		}
		r.Files = append(r.Files, a)
		if strings.HasPrefix(a, work) {
			data, err := os.ReadFile(a)
			if err != nil {
				return err
			}
			r.Saved[a] = string(data)
		}
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	dst := recordPath(os.Getenv(envStore), actionID(buildID))
	tmp, err := os.CreateTemp(filepath.Dir(dst), "record-")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
}

func run(cmd *exec.Cmd) int {
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "goesm -toolexec: %v\n", err)
		return 2
	}
	return 0
}
