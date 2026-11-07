// Command bytescompare compares byte slices through string conversions:
// string(a) == string(b), string(a) == "ascii" and bytes.Equal, of plain
// slices and of json.Marshal results.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type point struct {
	X, Y int
}

func marshalCompare() {
	b, err := json.Marshal(1)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b) == "1", string(b) != "1", string(b) == "2", len(b))

	c, err := json.Marshal(point{1, 2})
	if err != nil {
		panic(err)
	}
	d, err := json.Marshal(point{3, 4})
	if err != nil {
		panic(err)
	}
	e, err := json.Marshal(point{1, 2})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(c) == string(d), string(c) == string(e), string(c) != string(d))
	fmt.Println(string(c) == `{"X":1,"Y":2}`, len(c))
}

func sliceCompare() {
	a := []byte("abc")
	b := []byte("abd")
	c := append([]byte("ab"), 'c')
	fmt.Println(string(a) == string(b), string(a) == string(c), string(a) != string(b))
	fmt.Println(string(a) == "abc", "abc" == string(a), string(a[:2]) == "ab", string(a) == "")
	fmt.Println(bytes.Equal(a, c), bytes.Equal(a, b), bytes.Equal(nil, []byte{}))
	var empty []byte
	fmt.Println(string(empty) == "", string(empty) == string(a[:0]))
}

func main() {
	marshalCompare()
	sliceCompare()
}
