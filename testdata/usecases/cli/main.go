package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"
)

func main() {
	verbose := flag.Bool("v", false, "verbose")
	name := flag.String("name", "world", "name")
	n := flag.Int("n", 3, "count")
	dur := flag.Duration("d", time.Second, "dur")
	flag.Parse()
	fmt.Println("flags:", *verbose, *name, *n, *dur, flag.Args())
	fmt.Println("env:", os.Getenv("UC_ENV"), len(os.Args) > 0)
	dir, err := os.MkdirTemp("", "uc")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "x.txt"), []byte("hello\nworld\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "b", "y.csv"), []byte("name,age\nann,3\nbob,5\n"), 0o644)
	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files = append(files, rel+fmt.Sprint(d.IsDir()))
		return nil
	})
	sort.Strings(files)
	fmt.Println("walk:", files)
	f, err := os.Open(filepath.Join(dir, "a", "b", "y.csv"))
	if err != nil {
		log.Fatal(err)
	}
	recs, err := csv.NewReader(f).ReadAll()
	f.Close()
	fmt.Println("csv:", recs, err)
	fi, err := os.Stat(filepath.Join(dir, "a", "x.txt"))
	fmt.Println("stat:", fi.Size(), fi.Mode().IsRegular(), err)
	_, err = os.ReadFile(filepath.Join(dir, "nope"))
	fmt.Println("notexist:", os.IsNotExist(err))
	af, _ := os.OpenFile(filepath.Join(dir, "a", "x.txt"), os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintln(af, "more")
	af.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "a", "x.txt"))
	fmt.Printf("append: %q\n", b)
	os.Rename(filepath.Join(dir, "a", "x.txt"), filepath.Join(dir, "z.txt"))
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		fmt.Println("ent:", e.Name(), e.IsDir())
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	fmt.Println("glob:", len(matches))
	sc := bufio.NewScanner(os.Stdin)
	words := 0
	for sc.Scan() {
		words += len(strings.Fields(sc.Text()))
	}
	fmt.Println("stdin words:", words)
	re := regexp.MustCompile(`(\w+)@(\w+)\.com`)
	fmt.Println("re:", re.FindAllStringSubmatch("a@b.com, c@d.com", -1), re.ReplaceAllString("x@y.com", "$2"))
	t := template.Must(template.New("t").Funcs(template.FuncMap{"up": strings.ToUpper}).Parse("{{range .}}{{.Name | up}}={{.Age}};{{end}}\n"))
	t.Execute(os.Stdout, []struct {
		Name string
		Age  int
	}{{"ann", 3}, {"bob", 5}})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"ok": true, "n": 1.5})
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
		if a.Key == "time" {
			return slog.Attr{}
		}
		return a
	}})
	slog.New(h).Info("done", "files", len(files))
	jl := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
		if a.Key == "time" {
			return slog.Attr{}
		}
		return a
	}}))
	jl.Warn("json", "k", []int{1, 2})
	log.SetFlags(0)
	log.Println("to stderr")
	if *verbose {
		os.Exit(3)
	}
}
