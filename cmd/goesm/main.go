// Command goesm builds Go packages into ES modules.
//
//	goesm build [-o dist] [-split] [-minify] [-keep-ts dir] [-overlay file] [-toolexec cmd] [-v] ./main
//	goesm emit-ts [-o dir] [-dts] [-overlay file] [-toolexec cmd] [-v] ./main
//	goesm version
//
// Arguments are ordinary Go package patterns resolved by the go command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/goesm-dev/goesm/internal/build"
	"github.com/goesm-dev/goesm/internal/lower"
	"github.com/goesm-dev/goesm/internal/toolexec"
)

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  goesm build [-o dist] [-split] [-minify] [-keep-ts dir] [-overlay file] [-toolexec cmd] [-v] <package>...
  goesm emit-ts [-o dir] [-dts] [-overlay file] [-toolexec cmd] [-v] <package>...
  goesm version`)
	os.Exit(2)
}

func main() {
	// Run by the go command (or by a -toolexec program) during -toolexec.
	if toolexec.IsRecorder() {
		os.Exit(toolexec.Record(os.Args[1:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == toolexec.ShimCommand {
		os.Exit(toolexec.Shim(os.Args[2:]))
	}
	if len(os.Args) < 2 {
		usage()
	}
	cwd, _ := os.Getwd()
	switch os.Args[1] {
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		out := fs.String("o", "dist", "output directory")
		split := fs.Bool("split", false, "emit one ES module per Go package")
		minify := fs.Bool("minify", false, "minify output")
		keep := fs.String("keep-ts", "", "write generated TypeScript to this directory")
		ov := fs.String("overlay", "", "read file replacements from this JSON file (go build -overlay format)")
		tx := fs.String("toolexec", "", toolexecUsage)
		verbose := fs.Bool("v", false, "list standard library functions that are not supported yet")
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			usage()
		}
		res, err := build.Build(build.Options{Dir: cwd, Patterns: fs.Args(), OutDir: *out, Split: *split, Minify: *minify, TSDir: *keep, Overlay: readOverlay(*ov), Toolexec: toolexecProgram(*tx)})
		if err != nil {
			fail(err)
		}
		warn(res.Warnings, *verbose)
		for _, o := range res.Outputs {
			rel, _ := filepath.Rel(cwd, o)
			fmt.Println(rel)
		}
	case "emit-ts":
		fs := flag.NewFlagSet("emit-ts", flag.ExitOnError)
		out := fs.String("o", "goesm-ts", "output directory for TypeScript")
		ov := fs.String("overlay", "", "read file replacements from this JSON file (go build -overlay format)")
		tx := fs.String("toolexec", "", toolexecUsage)
		verbose := fs.Bool("v", false, "list standard library functions that are not supported yet")
		dts := fs.Bool("dts", false, "also write a declaration file (.d.ts) of each package's JS calling ABI")
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			usage()
		}
		overlay, err := build.ToolexecOverlay(cwd, toolexecProgram(*tx), readOverlay(*ov), fs.Args())
		if err != nil {
			fail(err)
		}
		l, err := build.LowerOverlay(cwd, overlay, fs.Args())
		if err != nil {
			fail(err)
		}
		warn(l.Warnings, *verbose)
		if err := build.WriteTS(*out, l.Mods); err != nil {
			fail(err)
		}
		if *dts {
			if err := build.WriteDTS(*out, l.Mods); err != nil {
				fail(err)
			}
		}
		for _, m := range l.Mods {
			fmt.Println(filepath.Join(*out, filepath.FromSlash(lower.ModuleFile(m.Path))))
		}
	case "version", "-version", "--version":
		fmt.Println("goesm", goesmVersion())
	default:
		usage()
	}
}

// goesmVersion reports the module version recorded by go install / go get
// -tool, followed by the Go toolchain goesm was built with (its go/types
// decides which Go it accepts).
func goesmVersion() string {
	v := "(devel)"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	if info.Main.Version != "" {
		v = info.Main.Version
	}
	v += " " + info.GoVersion
	return v
}

const toolexecUsage = "run compile commands through this program, as go build -toolexec does (default: -toolexec in GOFLAGS)"

// toolexecProgram is the -toolexec flag, or the -toolexec of GOFLAGS (how
// otelc's documentation plugs it into go build).
func toolexecProgram(flag string) string {
	if flag != "" {
		return flag
	}
	return toolexec.FromGOFLAGS(os.Getenv("GOFLAGS"))
}

func readOverlay(file string) map[string][]byte {
	if file == "" {
		return nil
	}
	m, err := build.ReadOverlay(file)
	if err != nil {
		fail(err)
	}
	return m
}

// warn names the standard library and dependency functions that the
// program may call but goesm cannot lower yet: they panic if called. -v
// gives each one's position and reason.
func warn(warnings []string, verbose bool) {
	if len(warnings) == 0 {
		return
	}
	if verbose {
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, w)
		}
	}
	fmt.Fprintf(os.Stderr, "goesm: %d standard library and dependency functions are not supported yet and panic if called", len(warnings))
	if !verbose {
		// A warning is "<pos>: <function> is not lowered and panics if called: <reason>".
		var names []string
		for _, w := range warnings {
			if i := strings.Index(w, " is not lowered"); i >= 0 {
				head := w[:i]
				names = append(names, head[strings.LastIndex(head, ": ")+2:])
			}
		}
		fmt.Fprintf(os.Stderr, ": %s (-v says why)", strings.Join(names, ", "))
	}
	fmt.Fprintln(os.Stderr)
}

func fail(err error) {
	var de *build.DiagError
	if errors.As(err, &de) {
		for _, l := range de.Lines {
			fmt.Fprintln(os.Stderr, l)
		}
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "goesm:", err)
	os.Exit(1)
}
