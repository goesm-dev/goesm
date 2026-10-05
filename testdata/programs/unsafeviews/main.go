// Command unsafeviews uses unsafe the ways libraries such as protobuf do,
// the ways goesm supports through pointer provenance: unsafe.String and
// unsafe.Slice of pointers from unsafe.StringData and unsafe.SliceData,
// header structs that mirror strings, slices and interfaces, structs read
// as structs of the same layout, and pointer variables read as other
// pointer types.
package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

type stringHeader struct {
	Data *byte
	Len  int
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

type ifaceHeader struct {
	Type unsafe.Pointer
	Data unsafe.Pointer
}

type point struct{ X, Y int }

type pair struct {
	A int
	B int
}

type info struct{ name string }

type state struct {
	_    [0]func()
	info *info
}

func (s *state) load() *info {
	return (*info)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.info))))
}

func (s *state) store(i *info) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.info)), unsafe.Pointer(i))
}

func main() {
	s := "hello, world"
	p := unsafe.StringData(s)
	fmt.Println(unsafe.String(p, 5), *p == 'h')
	fmt.Println(string(unsafe.Slice(p, 5)))

	b := []byte("bytes!")
	q := unsafe.SliceData(b)
	t := unsafe.Slice(q, 3)
	t[0] = 'B'
	fmt.Println(string(b), len(t), cap(t))
	fmt.Println(unsafe.String(q, len(b)))

	sh := (*stringHeader)(unsafe.Pointer(&s))
	fmt.Println(sh.Len, *sh.Data == 'h')
	slh := (*sliceHeader)(unsafe.Pointer(&b))
	fmt.Println(slh.Len, slh.Cap)

	var v any = &point{1, 2}
	ih := (*ifaceHeader)(unsafe.Pointer(&v))
	pt := (*point)(ih.Data)
	pt.X = 10
	fmt.Println(v.(*point).X, ih.Type != nil)

	pp := &point{3, 4}
	pr := (*pair)(unsafe.Pointer(pp))
	pr.B = 40
	fmt.Println(*pp, pr.A)
	// The pair read through pr is a pair in an interface too.
	var pi any = pair{3, 40}
	fmt.Println(pi == *pr, any(*pr).(pair).B)

	// A pointer variable holding its own address, through unsafe.Slice.
	var self unsafe.Pointer
	self = unsafe.Pointer(&self)
	sl := unsafe.Slice(&self, 1)
	sl[0] = nil
	fmt.Println(self == nil)

	var st state
	fmt.Println(st.load() == nil)
	st.store(&info{"stored"})
	fmt.Println(st.load().name)
}
