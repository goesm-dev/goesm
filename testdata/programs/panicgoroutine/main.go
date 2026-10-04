// Command panicgoroutine dies from a panic on another goroutine.
package main

import "os"

type code int

func main() {
	done := make(chan bool)
	go func() {
		os.Stdout.WriteString("in goroutine\n")
		panic(code(42))
	}()
	<-done
}
