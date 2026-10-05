// Command jsnames uses Go names that are reserved in TypeScript or name the
// JS globals generated code uses, calls functions held in slices and maps
// of struct fields, and reads a []byte as a string through unsafe.Pointer.
package main

import (
	"fmt"
	"unsafe"

	"programs/jsnames/consts"
)

// TypeScript's primitive type names, which protobuf-go uses for types.
type number struct{ kind int }
type boolean struct{ b bool }
type object struct{ o int }
type symbol int
type bigint struct{ x int64 }
type never struct{}
type unknown struct{}
type any_ = any

// Package-level names of JS globals (gjson's const Number, expr's lexer).
type Type int

const Number Type = 2

var BigInt = "big"
var Math = 3
var Error = fmt.Errorf("an error")

type Object struct{ Array []int }

func toInt(x int64) int       { return int(x) }
func toInt64(x int) int64     { return int64(x) }
func toUint32(x int64) uint32 { return uint32(x) }

type ops struct {
	fs  []func(int) int
	m   map[string]func() string
	two []func() (int, error)
}

func bytesString(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }
func stringBytes(s string) []byte { return *(*[]byte)(unsafe.Pointer(&s)) }

func main() {
	n := number{kind: 1}
	var s symbol = 2
	fmt.Println(n.kind, boolean{true}.b, object{3}.o, s, bigint{4}.x, never{}, unknown{}, any_(5))
	fmt.Println(Number, BigInt, Math, Error, Object{Array: []int{1}})
	fmt.Println(toInt(7), toInt64(8), toUint32(1<<33+9))

	o := &ops{
		fs:  []func(int) int{func(x int) int { return x + 1 }, func(x int) int { return x * 2 }},
		m:   map[string]func() string{"a": func() string { return "from map" }},
		two: []func() (int, error){func() (int, error) { return 42, nil }},
	}
	for i := range o.fs {
		fmt.Println("fs", i, o.fs[i](10))
	}
	fmt.Println(o.m["a"]())
	v, err := o.two[0]()
	fmt.Println(v, err)
	val := *o
	fmt.Println(val.fs[1](21))

	b := []byte("héllo")
	fmt.Println(bytesString(b), len(stringBytes("wörld")), string(stringBytes("wörld")))
	fmt.Println(consts.Legacy)
}
