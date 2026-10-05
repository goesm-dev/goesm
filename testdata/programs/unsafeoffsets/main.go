// Command unsafeoffsets does pointer arithmetic the way protobuf-go's fast
// path does: a message's fields reached by adding their reflect offsets to
// the message's address, the first field read through a pointer to the
// message, a []*T read as a slice of pointer-only structs, reflect.NewAt,
// unsafe.Add, and pointers round-tripped through uintptr.
package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

type state struct{ info *string }

type inner struct {
	A int32
	B string
}

type message struct {
	state  state
	ID     int64
	Name   string
	Tags   []string
	Inner  inner
	Arr    [3]uint16
	Kids   []*message
	Opt    *int32
	Scores map[string]float64
}

// pointer is protobuf's pointer: a struct holding only an unsafe.Pointer.
type pointer struct{ p unsafe.Pointer }

type offset uintptr

func (p pointer) Apply(f offset) pointer {
	return pointer{p: unsafe.Pointer(uintptr(p.p) + uintptr(f))}
}

func (p pointer) Int64() *int64           { return (*int64)(p.p) }
func (p pointer) Int32() *int32           { return (*int32)(p.p) }
func (p pointer) Uint16() *uint16         { return (*uint16)(p.p) }
func (p pointer) String() *string         { return (*string)(p.p) }
func (p pointer) StringSlice() *[]string  { return (*[]string)(p.p) }
func (p pointer) Int32Ptr() **int32       { return (**int32)(p.p) }
func (p pointer) PointerSlice() []pointer { return *(*[]pointer)(p.p) }
func (p pointer) Elem() pointer           { return pointer{p: *(*unsafe.Pointer)(p.p)} }
func (p pointer) AppendPointerSlice(v pointer) {
	*(*[]pointer)(p.p) = append(*(*[]pointer)(p.p), v)
}

func offsetOf(t reflect.Type, name string) offset {
	f, _ := t.FieldByName(name)
	return offset(f.Offset)
}

func stateOf(p unsafe.Pointer) *state { return (*state)(p) }

func main() {
	info := "message info"
	m := &message{ID: 7, Name: "seven", Tags: []string{"a", "b"}, Inner: inner{A: 1, B: "in"}, Arr: [3]uint16{10, 20, 30}}
	m.Kids = []*message{{ID: 1, Name: "one"}, {ID: 2, Name: "two"}}
	t := reflect.TypeFor[message]()
	p := pointer{p: unsafe.Pointer(m)}

	// Fields at reflect's offsets, read and written.
	fmt.Println(*p.Apply(offsetOf(t, "ID")).Int64(), *p.Apply(offsetOf(t, "Name")).String())
	*p.Apply(offsetOf(t, "ID")).Int64() = 70
	*p.Apply(offsetOf(t, "Name")).String() += "ty"
	fmt.Println(m.ID, m.Name)
	tags := p.Apply(offsetOf(t, "Tags")).StringSlice()
	*tags = append(*tags, "c")
	fmt.Println(m.Tags, len(*tags))

	// A nested struct's field and an array element, at summed offsets.
	in := p.Apply(offsetOf(t, "Inner")).Apply(offset(unsafe.Offsetof(inner{}.B)))
	fmt.Println(*in.String())
	el := p.Apply(offsetOf(t, "Arr")).Apply(offset(2 * unsafe.Sizeof(uint16(0))))
	*el.Uint16() = 33
	fmt.Println(m.Arr)

	// A pointer field, set through a pointer to it.
	n := int32(5)
	*p.Apply(offsetOf(t, "Opt")).Int32Ptr() = &n
	fmt.Println(*m.Opt)

	// The first field through a pointer to the message.
	stateOf(p.p).info = &info
	fmt.Println(*m.state.info, stateOf(unsafe.Pointer(m)) == &m.state)

	// []*message as []pointer, aliased with the field.
	kids := p.Apply(offsetOf(t, "Kids"))
	for _, k := range kids.PointerSlice() {
		fmt.Println(*k.Apply(offsetOf(t, "ID")).Int64(), *k.Apply(offsetOf(t, "Name")).String())
	}
	kids.AppendPointerSlice(pointer{p: unsafe.Pointer(&message{ID: 3, Name: "three"})})
	fmt.Println(len(m.Kids), m.Kids[2].Name)
	// Appends that fit in the capacity, as protobuf decodes repeated
	// message fields.
	for i := range 6 {
		kids.AppendPointerSlice(pointer{p: unsafe.Pointer(&message{ID: int64(4 + i), Name: fmt.Sprint("kid ", 4+i)})})
	}
	for _, k := range m.Kids {
		fmt.Print(k.ID, " ", k.Name, "; ")
	}
	fmt.Println(len(m.Kids))
	fmt.Println(*kids.PointerSlice()[0].Apply(offsetOf(t, "Name")).String())

	// reflect.NewAt over a field's address.
	v := reflect.NewAt(reflect.TypeFor[string](), p.Apply(offsetOf(t, "Name")).p)
	v.Elem().SetString("set by reflect")
	fmt.Println(m.Name, v.Type())
	mv := reflect.NewAt(t, p.p)
	fmt.Println(mv.Elem().FieldByName("ID").Int(), mv.Interface().(*message) == m)

	// unsafe.Add, and an address round-tripped through uintptr.
	fmt.Println(*(*int32)(unsafe.Add(unsafe.Pointer(&m.Inner), unsafe.Offsetof(inner{}.A))))
	addr := uintptr(unsafe.Pointer(m))
	back := (*message)(unsafe.Pointer(addr))
	fmt.Println(back == m, back.Name)

	// A pointer to a field compared with one computed from an offset.
	fmt.Println(unsafe.Pointer(&m.Name) == p.Apply(offsetOf(t, "Name")).p)
}
