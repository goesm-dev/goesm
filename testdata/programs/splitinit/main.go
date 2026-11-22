// Command splitinit imports two packages that do not import each other and
// register themselves while they initialize, the first one after waiting.
// Go initializes them one after the other, in the order of their paths;
// the second one must not start while the first one waits, also when each
// package is a module of its own (the split file: goesm build -split).
package main

import (
	"fmt"

	"programs/splitinit/alpha"
	"programs/splitinit/beta"
	"programs/splitinit/registry"
)

func main() {
	fmt.Printf("alpha after %q, beta after %q, all %q\n", alpha.Seen, beta.Seen, registry.Names())
}
