// Command byteslices exercises []byte across the ways a slice gets its
// backing array (make, []byte(s), append growth, slice literals, Go arrays)
// and the operations that mix them.
package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

func main() {
	b := make([]byte, 4, 8)
	copy(b, "abcd")
	p := (*[4]byte)(b) // aliases b
	p[0] = 'X'
	a := [4]byte(b) // copies
	a[1] = 'Y'
	fmt.Println(string(b), string(a[:]), *p)

	// Overlapping copies, both directions.
	c := []byte("0123456789")
	copy(c[2:], c)
	fmt.Println(string(c))
	c = []byte("0123456789")
	copy(c, c[3:])
	fmt.Println(string(c))

	// Append from a Go array, a literal and a string, aliasing included.
	var arr [5]byte
	s := arr[:2]
	s = append(s, 'q')
	fmt.Println(arr, s)
	lit := []byte{1, 2, 3}
	lit = append(lit, lit...)
	lit = append(lit[:2], lit[1:]...)
	fmt.Println(lit, len(lit), cap(lit) >= len(lit))
	var grown []byte
	for i := range 300 {
		grown = append(grown, byte(i))
	}
	fmt.Println(len(grown), grown[255], grown[299], string(append([]byte("go"), "esm"...)))

	// Values wrap like Go bytes.
	w := make([]byte, 2)
	x := 250
	w[0] = byte(x + 10)
	w[1] -= 1
	fmt.Println(w)

	// The standard library and reflection.
	var buf bytes.Buffer
	for i := range 1000 {
		fmt.Fprintf(&buf, "%d,", i)
	}
	fmt.Println(buf.Len(), strings.Count(buf.String(), ","), bytes.Index(buf.Bytes(), []byte("999")))
	rv := reflect.ValueOf([]byte("hello"))
	fmt.Println(rv.Index(1).Uint(), string(rv.Bytes()), reflect.DeepEqual([]byte("hi"), append([]byte(nil), "hi"...)))
	rs := reflect.MakeSlice(reflect.TypeOf([]byte(nil)), 3, 3)
	rs.Index(0).SetUint(7)
	rs = reflect.Append(rs, reflect.ValueOf(byte(9)))
	fmt.Println(rs.Interface())
	ap := reflect.ValueOf(b).Convert(reflect.TypeOf((*[4]byte)(nil))).Interface().(*[4]byte)
	ap[3] = 'Z'
	fmt.Println(string(b))

	// unsafe views.
	u := []byte("view")
	str := unsafe.String(unsafe.SliceData(u), len(u))
	fmt.Println(str, unsafe.Slice(unsafe.StringData(str), 2))
	fmt.Println(bytes.Equal(bytes.ToUpper([]byte("abc")), []byte("ABC")), bytes.Repeat([]byte("ab"), 3))

	// range over a conversion of a string ranges over its bytes.
	text := "héllo, 世界"
	sum := 0
	for i, c := range []byte(text) {
		sum += i * int(c)
	}
	fmt.Println(sum, len([]byte(text)))
	type octet byte
	var last octet
	var at int
	for at, last = range []octet(text + "!") {
		if last == ',' {
			break
		}
	}
	fmt.Println(at, last)
	n := 0
	for range []byte(strings.Repeat("ab", 3)) {
		n++
	}
	fmt.Println(n)
}
