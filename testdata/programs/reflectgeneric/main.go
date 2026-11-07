// A function stored in a standard library struct's function field that
// the program reads only through reflection and calls after a generic
// function asserts the value's Interface() to its type parameter.
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
)

func as[T any](x any) T { return x.(T) }

func main() {
	req, err := http.NewRequest("GET", "http://localhost/", nil)
	if err != nil {
		panic(err)
	}
	field := reflect.ValueOf(http.DefaultTransport).Elem().FieldByName("Proxy")
	proxy := as[func(*http.Request) (*url.URL, error)](field.Interface())
	u, err := proxy(req)
	fmt.Println(u == nil, err)
}
