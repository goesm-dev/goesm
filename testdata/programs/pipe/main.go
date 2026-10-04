// Command pipe streams through io.Pipe: the Write and Read calls made
// through io.Writer and io.Reader block, so the code reaching them must be
// async, while plain writes to os.Stdout through io.Writer stay synchronous.
package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

type upper struct{ w io.Writer }

func (u upper) Write(p []byte) (int, error) {
	return u.w.Write([]byte(strings.ToUpper(string(p))))
}

func produce(w io.WriteCloser) {
	for _, s := range []string{"alpha\n", "beta\n", "gamma\n"} {
		io.WriteString(w, s)
	}
	w.Close()
}

func main() {
	var out io.Writer = os.Stdout
	io.WriteString(out, "direct\n")

	pr, pw := io.Pipe()
	go produce(pw)
	n, err := io.Copy(upper{out}, pr)
	if err != nil {
		panic(err)
	}
	println("copied", n)

	pr2, pw2 := io.Pipe()
	go func() {
		bw := bufio.NewWriter(pw2)
		bw.WriteString("one\ntwo\n")
		bw.Flush()
		pw2.Close()
	}()
	sc := bufio.NewScanner(pr2)
	for sc.Scan() {
		os.Stdout.WriteString("line: " + sc.Text() + "\n")
	}
}
