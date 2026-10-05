//go:build js && go1.23

package main

import "example.com/bench/kernels"

// Pull needs iter, which GopherJS's Go 1.21 lacks.
func init() {
	byNameAsync["Pull"] = kernels.Pull
}
