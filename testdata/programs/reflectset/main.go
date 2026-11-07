// A function field that only reflection stores a function into: the
// function, which Go code put in an interface value, waits on a channel,
// so the call through the field waits too.
package main

import (
	"fmt"
	"reflect"
)

type S struct {
	F func() int
}

func main() {
	ch := make(chan int, 1)
	ch <- 41
	block := func() int { return <-ch }
	var s S
	reflect.ValueOf(&s).Elem().FieldByName("F").Set(reflect.ValueOf(block))
	fmt.Println(s.F() + 1)
}
