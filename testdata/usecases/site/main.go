// Command site is a static site generator, the kind of build tool that runs
// under Node.js: Markdown with YAML front matter to HTML (goldmark), a TOML
// configuration, content hashes and gzip, reading and writing files.
//
// Usage: site <dir with config.toml and content/> <output dir>
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"gopkg.in/yaml.v3"
)

type front struct {
	Title string   `yaml:"title"`
	Tags  []string `yaml:"tags"`
	Draft bool     `yaml:"draft"`
}

type config struct {
	Site struct {
		Name string
		Base string
	}
	Langs []string
}

type page struct {
	Path  string   `json:"path"`
	Title string   `json:"title"`
	Tags  []string `json:"tags,omitempty"`
	Hash  string   `json:"hash"`
	Gzip  int      `json:"gzipBytes"`
}

func main() {
	log.SetFlags(0)
	src, out := os.Args[1], os.Args[2]
	var cfg config
	if _, err := toml.DecodeFile(filepath.Join(src, "config.toml"), &cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("site %s at %s, languages %v\n", cfg.Site.Name, cfg.Site.Base, cfg.Langs)
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	paths, err := filepath.Glob(filepath.Join(src, "content", "*.md"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(paths)
	var pages []page
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			log.Fatal(err)
		}
		parts := strings.SplitN(string(b), "---\n", 3)
		var fm front
		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			log.Fatalf("%s: %v", p, err)
		}
		if fm.Draft {
			fmt.Println("skip draft", filepath.Base(p))
			continue
		}
		var html bytes.Buffer
		fmt.Fprintf(&html, "<title>%s</title>\n", fm.Title)
		if err := md.Convert([]byte(parts[2]), &html); err != nil {
			log.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".md") + ".html"
		if err := os.WriteFile(filepath.Join(out, name), html.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
		var gz bytes.Buffer
		w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		w.Write(html.Bytes())
		w.Close()
		r, err := gzip.NewReader(bytes.NewReader(gz.Bytes()))
		if err != nil {
			log.Fatal(err)
		}
		back, _ := io.ReadAll(r)
		if !bytes.Equal(back, html.Bytes()) {
			log.Fatal("gzip round trip differs")
		}
		sum := sha256.Sum256(html.Bytes())
		pages = append(pages, page{Path: cfg.Site.Base + name, Title: fm.Title, Tags: fm.Tags, Hash: hex.EncodeToString(sum[:8]), Gzip: gz.Len()})
		fmt.Print(html.String())
	}
	manifest, _ := json.MarshalIndent(pages, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), manifest, 0o644); err != nil {
		log.Fatal(err)
	}
	y, _ := yaml.Marshal(map[string]any{"pages": len(pages), "langs": cfg.Langs})
	fmt.Printf("%s\n%s", manifest, y)
}
