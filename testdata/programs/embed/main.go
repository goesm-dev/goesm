// Command embed prints files embedded with //go:embed as a string, a []byte
// and an embed.FS.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed static/hello.txt
var hello string

//go:embed static/sub/data.bin
var data []byte

//go:embed static
var site embed.FS

//go:embed all:static/.hidden "static/sub/with space.txt"
var extra embed.FS

func main() {
	fmt.Printf("%q\n", hello)
	fmt.Println(data, len(data))
	for _, fsys := range []embed.FS{site, extra} {
		fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Println("error:", err)
				return err
			}
			if d.IsDir() {
				fmt.Println("dir ", path)
				return nil
			}
			b, _ := fs.ReadFile(fsys, path)
			info, _ := d.Info()
			fmt.Printf("file %s %d %q\n", path, info.Size(), strings.TrimSpace(string(b)))
			return nil
		})
	}
	_, err := site.Open("static/missing.txt")
	fmt.Println(err)
	entries, _ := site.ReadDir("static/sub")
	for _, e := range entries {
		fmt.Println("entry", e.Name(), e.IsDir())
	}
}
