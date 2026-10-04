// Command goexitmain calls runtime.Goexit in main: deferred calls run, the
// other goroutines go on, and once they are done the program crashes.
package main

import (
	"os"
	"runtime"
)

func main() {
	ch := make(chan int)
	go func() {
		for v := range ch {
			os.Stdout.WriteString("worker got " + string(rune('0'+v)) + "\n")
		}
		os.Stdout.WriteString("worker done\n")
	}()
	go func() {
		for i := 1; i <= 3; i++ {
			ch <- i
		}
		close(ch)
	}()
	defer os.Stdout.WriteString("main: deferred call runs\n")
	runtime.Goexit()
	os.Stdout.WriteString("not reached\n")
}
