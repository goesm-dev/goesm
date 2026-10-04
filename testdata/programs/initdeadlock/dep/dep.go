// Package dep blocks forever while it is initialized.
package dep

import "os"

var Ready = true

func init() {
	os.Stdout.WriteString("dep: init\n")
	<-make(chan int)
}
