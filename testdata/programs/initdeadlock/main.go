// Command initdeadlock imports a package whose init function blocks forever:
// the program reports a deadlock before main runs.
package main

import (
	"os"

	"programs/initdeadlock/dep"
)

func main() {
	if dep.Ready {
		os.Stdout.WriteString("main must not run\n")
	}
}
