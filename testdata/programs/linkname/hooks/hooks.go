// Package hooks is reached only through //go:linkname.
package hooks

import (
	"fmt"
	"strings"
	_ "unsafe"
)

var calls []string

func init() {
	calls = append(calls, "init")
}

// Before rewrites its caller's argument.
func Before(name string, n *int) string {
	calls = append(calls, "before "+name)
	*n += len(calls)
	return strings.ToUpper(name)
}

func after(name string) {
	calls = append(calls, "after "+name)
}

//go:linkname pushed main.pushed
func pushed(x int) int

func Greet() string {
	return fmt.Sprintf("%s; pushed(4)=%d", strings.Join(calls, ", "), pushed(4))
}
