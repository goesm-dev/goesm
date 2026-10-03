// Command deadlock blocks forever on a channel no goroutine sends on.
package main

import "os"

func main() {
	os.Stdout.WriteString("waiting\n")
	ch := make(chan int)
	<-ch
}
