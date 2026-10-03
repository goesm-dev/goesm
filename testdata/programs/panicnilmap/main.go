// Command panicnilmap dies with a runtime error.
package main

import "os"

func main() {
	os.Stdout.WriteString("before\n")
	var m map[string]int
	m["x"] = 1
	os.Stdout.WriteString("after (must not print)\n")
}
