package main

import (
	"errors"
	"fmt"
	"syscall/js"

	"github.com/goesm-dev/goesm/promise"
)

// The module is imported on the first call of these functions, not when
// the program starts.
//
//goesm:import "./lazy.ts" * lazy
func lazyModule() (js.Value, error)

//goesm:import "./lazy.ts" twice lazy
func lazyTwice(n int) (int, error)

//goesm:import "./lazy.ts" later lazy
func lazyLater(s string) string

// promises waits for Promises that syscall/js calls return, with
// promise.Await, and calls lazily imported functions.
func promises() {
	fmt.Println("lazy loaded before:", js.Global().Get("lazyLoaded").Truthy())
	n, err := lazyTwice(21)
	fmt.Println(n, err, "lazy loaded after:", js.Global().Get("lazyLoaded").Truthy())
	m, err := lazyModule()
	fmt.Println(m.Get("name").String(), err, lazyLater("later"))

	v, err := promise.Await(js.Global().Get("Promise").Call("resolve", "resolved"))
	fmt.Println(v.String(), err)
	v, err = promise.Await(js.ValueOf(7))
	fmt.Println(v.Int(), err)
	_, err = promise.Await(js.Global().Get("Promise").Call("reject", js.Global().Get("DOMException").New("cancelled", "AbortError")))
	var jerr js.Error
	fmt.Println(errors.As(err, &jerr) && jerr.Get("name").String() == "AbortError", err)
	_, err = promise.Await(js.Global().Get("Promise").Call("reject", "plain reason"))
	fmt.Println(errors.As(err, &jerr), err)
}
