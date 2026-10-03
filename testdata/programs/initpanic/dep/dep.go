// Package dep panics while it is initialized.
package dep

import "os"

var Ready = setup()

func setup() bool {
	os.Stdout.WriteString("dep: variables initialized\n")
	return true
}

func init() {
	os.Stdout.WriteString("dep: init\n")
	panic("dep: init failed")
}
