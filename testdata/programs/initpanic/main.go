// Command initpanic imports a package whose init function panics: the
// program crashes before main runs.
package main

import (
	"os"

	"programs/initpanic/dep"
)

func main() {
	if dep.Ready {
		os.Stdout.WriteString("main must not run\n")
	}
}
