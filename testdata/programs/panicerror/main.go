// Command panicerror panics with values of several kinds; the last one is
// not recovered.
package main

import (
	"errors"
	"os"
)

type code int

func try(v any) {
	defer func() {
		r := recover()
		if e, ok := r.(error); ok {
			os.Stdout.WriteString("recovered error: " + e.Error() + "\n")
		} else if s, ok := r.(string); ok {
			os.Stdout.WriteString("recovered string: " + s + "\n")
		} else {
			os.Stdout.WriteString("recovered other\n")
		}
	}()
	panic(v)
}

func main() {
	try("str")
	try(errors.New("boom"))
	try(code(7))
	defer os.Stdout.WriteString("deferred runs before the crash\n")
	panic(errors.New("unrecovered"))
}
