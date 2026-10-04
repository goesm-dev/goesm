// Command linkname calls functions of another package through bodyless
// //go:linkname declarations, as compile-time instrumentation generates them
// (pulls), and provides one of its own by name (a push).
package main

import (
	"fmt"
	_ "unsafe"

	_ "programs/linkname/hooks"
)

//go:linkname before programs/linkname/hooks.Before
func before(name string, n *int) string

//go:linkname after programs/linkname/hooks.after
func after(name string)

//go:linkname greet programs/linkname/hooks.Greet
func greet() string

// pushed is called by package hooks under the name main.pushed.
//
//go:linkname pushed main.pushed
func pushed(x int) int { return x * 10 }

func work(name string) {
	n := 1
	tag := before(name, &n)
	defer after(name)
	fmt.Println("work", name, n, tag)
}

func main() {
	work("a")
	work("b")
	fmt.Println(greet())
}
