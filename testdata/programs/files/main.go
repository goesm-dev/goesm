// Command files writes, reads, lists and removes files through package os.
package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func say(parts ...string) { os.Stdout.WriteString(strings.Join(parts, " ") + "\n") }

func e(err error) string {
	if err == nil {
		return "<nil>"
	}
	return "error"
}

func main() {
	dir, err := os.MkdirTemp("", "goesm-files-")
	if err != nil {
		say("MkdirTemp:", err.Error())
		return
	}
	defer os.RemoveAll(dir)
	name := filepath.Join(dir, "a.txt")
	say(e(os.WriteFile(name, []byte("hello\nworld\n"), 0o644)))
	b, err := os.ReadFile(name)
	say(strconv.Quote(string(b)), e(err))

	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0)
	say(e(err))
	f.WriteString("more\n")
	say(e(f.Close()))

	f, err = os.Open(name)
	say(e(err))
	buf := make([]byte, 5)
	n, err := io.ReadFull(f, buf)
	say(strconv.Itoa(n), e(err), string(buf))
	rest, err := io.ReadAll(f)
	say(strconv.Quote(string(rest)), e(err))
	st, err := f.Stat()
	say(st.Name(), strconv.FormatInt(st.Size(), 10), strconv.FormatBool(st.IsDir()), strconv.FormatBool(st.Mode().IsRegular()), e(err))
	f.Close()

	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), nil, 0o600)
	entries, err := os.ReadDir(dir)
	for _, d := range entries {
		say(d.Name(), strconv.FormatBool(d.IsDir()))
	}
	say(e(err))
	say(e(os.Rename(name, filepath.Join(dir, "c.txt"))))
	_, err = os.Stat(name)
	say(strconv.FormatBool(os.IsNotExist(err)))
	_, err = os.Open(filepath.Join(dir, "missing"))
	say(strconv.FormatBool(err != nil), strconv.FormatBool(os.IsNotExist(err)))
	say(e(os.Remove(filepath.Join(dir, "c.txt"))))
}
