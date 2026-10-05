// Package jsimportresolve imports a package that is not installed, which
// esbuild cannot resolve.
package main

//goesm:import "no-such-package" f
func f() int

func main() { println(f()) }
