// A function stored in a standard library struct's function field that
// the program reads only through reflection and calls after a type
// assertion of the value's Interface().
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
)

func main() {
	req, err := http.NewRequest("GET", "http://localhost/", nil)
	if err != nil {
		panic(err)
	}
	field := reflect.ValueOf(http.DefaultTransport).Elem().FieldByName("Proxy")
	proxy := field.Interface().(func(*http.Request) (*url.URL, error))
	u, err := proxy(req)
	fmt.Println(u == nil, err)
}
