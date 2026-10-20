// A program that swaps os.Stdout: fmt.Println must follow the variable.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("to stdout")
	old := os.Stdout
	os.Stdout = os.Stderr
	fmt.Println("to stderr")
	fmt.Printf("%s\n", "printf to stderr")
	os.Stdout = old
	fmt.Println("back to stdout")
	fmt.Fprintln(os.Stderr, "stderr is fixed")
}
