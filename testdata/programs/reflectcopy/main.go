// A function field that reflection sets from another struct's field of the
// same type: the function there waits on a channel, so the call through
// the field set waits too.
package main

import (
	"fmt"
	"reflect"
)

type From struct {
	G func() int
}

type To struct {
	G func() int
}

func main() {
	ch := make(chan int, 1)
	ch <- 2
	from := &From{G: func() int { return <-ch }}
	var to To
	reflect.ValueOf(&to).Elem().Field(0).Set(reflect.ValueOf(from).Elem().Field(0))
	fmt.Println(to.G() + 1)
}
