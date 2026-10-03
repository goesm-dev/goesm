// Command goesm builds Go packages into ES modules.
//
//	goesm build [-o dist] [-split] [-minify] [-keep-ts dir] ./main
//	goesm emit-ts [-o dir] ./main
//
// Arguments are ordinary Go package patterns resolved by the go command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goesm-dev/goesm/internal/build"
)

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  goesm build [-o dist] [-split] [-minify] [-keep-ts dir] <package>
  goesm emit-ts [-o dir] <package>`)
	os.Exit(2)
}

func main() {
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
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			usage()
		}
		res, err := build.Build(build.Options{Dir: cwd, Patterns: fs.Args(), OutDir: *out, Split: *split, Minify: *minify, TSDir: *keep})
		if err != nil {
			fail(err)
		}
		for _, o := range res.Outputs {
			rel, _ := filepath.Rel(cwd, o)
			fmt.Println(rel)
		}
	case "emit-ts":
		fs := flag.NewFlagSet("emit-ts", flag.ExitOnError)
		out := fs.String("o", "goesm-ts", "output directory for TypeScript")
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			usage()
		}
		mods, _, err := build.Lower(cwd, fs.Args())
		if err != nil {
			fail(err)
		}
		if err := build.WriteTS(*out, mods); err != nil {
			fail(err)
		}
		for _, m := range mods {
			fmt.Printf("%s/go/%s.ts\n", *out, m.Path)
		}
	default:
		usage()
	}
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
