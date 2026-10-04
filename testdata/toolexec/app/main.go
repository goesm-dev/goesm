// Command app prints what the rewriter changes when it builds it.
package main

import (
	"errors"
	"fmt"
)

var messages []string

func main() {
	fmt.Println("original")
	fmt.Println(messages)
	fmt.Println(errors.New("boom"))
}
