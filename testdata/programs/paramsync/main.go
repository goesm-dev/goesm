// Functions that block only through their arguments: fmt.Fprintf blocks
// when it writes to an io.PipeWriter, and a function calling its func()
// parameter blocks when that function does. Calls whose arguments do not
// block call synchronous clones, so fmt.Sprintf, and the functions below
// that only pass non-blocking writers and functions, stay synchronous.
package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type point struct{ x, y int }

func (p point) Format(f fmt.State, verb rune) { fmt.Fprintf(f, "(%d,%d)", p.x, p.y) }

func label(p point) string { return fmt.Sprintf("point %v", p) }

func apply(f func()) { f() }

func twice(f func()) {
	apply(f)
	if f != nil {
		apply(f)
	}
}

func write(w io.Writer, s string) {
	if sw, ok := w.(io.StringWriter); ok {
		sw.WriteString(s)
		return
	}
	w.Write([]byte(s))
}

func count() int {
	n := 0
	twice(func() { n++ })
	var once sync.Once
	g := func() { n += 10 }
	once.Do(g)
	once.Do(g)
	return n
}

func digest(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil)[:4])
}

func build() string {
	var b strings.Builder
	write(&b, "built")
	fmt.Fprintf(&b, " %d", 42)
	return b.String()
}

func main() {
	fmt.Println(label(point{1, 2}), count(), digest("goesm"), build())

	pr, pw := io.Pipe()
	go func() {
		fmt.Fprintf(pw, "piped %v\n", point{3, 4})
		write(pw, "written\n")
		pw.Close()
	}()
	io.Copy(os.Stdout, pr)

	c := make(chan int, 1)
	twice(func() {
		go func() { c <- 1 }()
		fmt.Println("received", <-c)
	})
	init := sync.OnceFunc(func() { fmt.Println("once") })
	init()
	init()
}
