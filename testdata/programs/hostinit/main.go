// Command hostinit uses what goesm sets up only when a program needs it: the
// GODEBUG settings, read at the first lookup of one (TestGODEBUG runs it
// with GODEBUG set), and the host's file system with the flags of its
// open calls.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
)

func main() {
	// urlmaxqueryparams limits the parameters of a query (10000 unless
	// GODEBUG says otherwise).
	q, err := url.ParseQuery("a=1&b=2&c=3")
	fmt.Println(len(q), err)

	dir, err := os.MkdirTemp("", "hostinit")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	name := filepath.Join(dir, "f")
	for _, flag := range []int{os.O_WRONLY | os.O_CREATE | os.O_EXCL, os.O_WRONLY | os.O_CREATE | os.O_EXCL, os.O_WRONLY | os.O_APPEND, os.O_RDWR | os.O_TRUNC} {
		f, err := os.OpenFile(name, flag, 0o644)
		if err != nil {
			fmt.Println(errors.Is(err, fs.ErrExist))
			continue
		}
		_, err = f.WriteString("ab")
		fmt.Println(err, f.Close())
		b, err := os.ReadFile(name)
		fmt.Printf("%q %v\n", b, err)
	}
	_, err = os.OpenFile(dir, os.O_RDONLY, 0)
	fmt.Println(err)
}
