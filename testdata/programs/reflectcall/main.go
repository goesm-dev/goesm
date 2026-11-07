// A function stored in a standard library struct's function field that
// the program reads only through reflection and calls through
// reflect.Value.Call.
package main

import (
	"fmt"
	"net/http"
	"reflect"
)

func main() {
	req, err := http.NewRequest("GET", "http://localhost/", nil)
	if err != nil {
		panic(err)
	}
	field := reflect.ValueOf(http.DefaultTransport).Elem().FieldByName("Proxy")
	out := field.Call([]reflect.Value{reflect.ValueOf(req)})
	fmt.Println(out[0].IsNil(), out[1].IsNil())
}
