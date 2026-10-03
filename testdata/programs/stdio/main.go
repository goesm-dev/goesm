// Command stdio writes to standard output and standard error through os and
// the print builtins.
package main

import (
	"math"
	"os"
)

type point struct{ x, y int }

func main() {
	os.Stdout.WriteString("hello, stdout\n")
	os.Stderr.Write([]byte("hello, stderr\n"))
	for i := 0; i < 3; i++ {
		os.Stdout.WriteString("line ")
		os.Stdout.Write([]byte{byte('0' + i), '\n'})
	}
	println("println:", 42, -7, true, false, "str")
	println(3.5, -0.000123, 1e300, float32(0.1), math.Inf(1), math.Inf(-1), math.NaN())
	println(0.0, math.Copysign(0, -1), 1.0, 123456789.0)
	println(complex(1.5, -2))
	print("print:", 1, 2, "\n")
	var p *point
	var m map[string]int
	var e error
	var s []int
	println(p, m, e, s)
	os.Stdout.WriteString("done\n")
}

func init() {
	println(float32(1)/3, float32(16777216), 1234567.0, 123456.0, 0.0001, 0.00001, 1e21, 100.0)
	println(complex64(complex(1.0/3, math.NaN())), complex(math.Inf(-1), 0))
}
