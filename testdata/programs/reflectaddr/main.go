// Command reflectaddr checks that pointers reflect hands out (Value.Addr of
// fields and elements) are the same pointers & gives, and that setting
// through addressable Values reaches the variable.
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type T struct {
	A int
	B string
	C [3]int
	D []float64
}

func main() {
	t := &T{A: 1, B: "x", C: [3]int{1, 2, 3}, D: []float64{1.5, 2.5}}
	v := reflect.ValueOf(t).Elem()
	pa := v.Field(0).Addr().Interface().(*int)
	fmt.Println(pa == &t.A, v.Field(0).Addr().Interface() == v.Field(0).Addr().Interface())
	pc := v.Field(2).Index(1).Addr().Interface().(*int)
	fmt.Println(pc == &t.C[1])
	pd := v.Field(3).Index(0).Addr().Interface().(*float64)
	fmt.Println(pd == &t.D[0])
	*pa = 7
	v.Field(1).SetString("y")
	v.Field(2).Index(2).SetInt(9)
	v.Field(3).Index(1).SetFloat(4.5)
	fmt.Println(*t)
	m := map[*int]string{&t.A: "a"}
	fmt.Println(m[v.Field(0).Addr().Interface().(*int)])

	var u T
	if err := json.Unmarshal([]byte(`{"A":3,"B":"z","C":[4,5,6],"D":[0.5]}`), &u); err != nil {
		panic(err)
	}
	fmt.Println(u)
}
