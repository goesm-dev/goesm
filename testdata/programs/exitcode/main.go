// Command exitcode exits with a status code; deferred calls do not run.
package main

import "os"

func main() {
	defer os.Stdout.WriteString("deferred (must not print)\n")
	os.Stdout.WriteString("exiting\n")
	os.Exit(3)
}
