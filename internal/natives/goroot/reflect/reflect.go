//go:build goesm

// Package reflect is goesm's replacement for the standard library's reflect
// package. A Type is a @goesm/runtime type descriptor (*rtype); a Value
// holds a JS value, or for an addressable Value a goesm pointer to it (the
// object itself for structs and arrays, a Cell or field / element pointer
// otherwise). The descriptor and value operations that depend on goesm's
// representation are natives (runtime/src/natives.ts); everything else is
// ordinary Go, following the standard library's reflect.
//
// Not supported: Value.Call and MakeFunc of functions that block, Select,
// blocking Send and Recv, StructOf, NewAt, SliceAt, and the address-based
// operations (UnsafeAddr, InterfaceData).
package reflect

import (
	"iter"
	"strconv"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// A Kind represents the specific kind of type that a Type represents.
type Kind uint

const (
	Invalid Kind = iota
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	Array
	Chan
	Func
	Interface
	Map
	Pointer
	Slice
	String
	Struct
	UnsafePointer
)

// Ptr is the old name for the Pointer kind.
const Ptr = Pointer

var kindNames = []string{
	Invalid:       "invalid",
	Bool:          "bool",
	Int:           "int",
	Int8:          "int8",
	Int16:         "int16",
	Int32:         "int32",
	Int64:         "int64",
	Uint:          "uint",
	Uint8:         "uint8",
	Uint16:        "uint16",
	Uint32:        "uint32",
	Uint64:        "uint64",
	Uintptr:       "uintptr",
	Float32:       "float32",
	Float64:       "float64",
	Complex64:     "complex64",
	Complex128:    "complex128",
	Array:         "array",
	Chan:          "chan",
	Func:          "func",
	Interface:     "interface",
	Map:           "map",
	Pointer:       "ptr",
	Slice:         "slice",
	String:        "string",
	Struct:        "struct",
	UnsafePointer: "unsafe.Pointer",
}

// String returns the name of k.
func (k Kind) String() string {
	if uint(k) < uint(len(kindNames)) {
		return kindNames[uint(k)]
	}
	return "kind" + strconv.Itoa(int(k))
}

// ChanDir represents a channel type's direction.
type ChanDir int

const (
	RecvDir ChanDir = 1 << iota
	SendDir
	BothDir = RecvDir | SendDir
)

func (d ChanDir) String() string {
	switch d {
	case SendDir:
		return "chan<-"
	case RecvDir:
		return "<-chan"
	case BothDir:
		return "chan"
	}
	return "ChanDir" + strconv.Itoa(int(d))
}

// Type is the representation of a Go type.
type Type interface {
	Align() int
	FieldAlign() int
	Method(int) Method
	Methods() iter.Seq[Method]
	MethodByName(string) (Method, bool)
	NumMethod() int
	Name() string
	PkgPath() string
	Size() uintptr
	String() string
	Kind() Kind
	Implements(u Type) bool
	AssignableTo(u Type) bool
	ConvertibleTo(u Type) bool
	Comparable() bool
	Bits() int
	ChanDir() ChanDir
	IsVariadic() bool
	Elem() Type
	Field(i int) StructField
	Fields() iter.Seq[StructField]
	FieldByIndex(index []int) StructField
	FieldByName(name string) (StructField, bool)
	FieldByNameFunc(match func(string) bool) (StructField, bool)
	In(i int) Type
	Ins() iter.Seq[Type]
	Key() Type
	Len() int
	NumField() int
	NumIn() int
	NumOut() int
	Out(i int) Type
	Outs() iter.Seq[Type]
	OverflowComplex(x complex128) bool
	OverflowFloat(x float64) bool
	OverflowInt(x int64) bool
	OverflowUint(x uint64) bool
	CanSeq() bool
	CanSeq2() bool
	common() *rtype
}

// rtype is a runtime type descriptor; Go code never looks inside it. It is
// not zero-size, so distinct descriptors are distinct pointers (goesm, like
// gc, makes all pointers to zero-size values equal).
type rtype struct {
	_ [0]func()
	_ uintptr
}

// Method represents a single method.
type Method struct {
	Name    string
	PkgPath string
	Type    Type  // method type
	Func    Value // func with receiver as first argument
	Index   int   // index for Type.Method
}

// IsExported reports whether the method is exported.
func (m Method) IsExported() bool { return m.PkgPath == "" }

// A StructField describes a single field in a struct.
type StructField struct {
	Name      string
	PkgPath   string
	Type      Type      // field type
	Tag       StructTag // field tag string
	Offset    uintptr   // offset within struct, in bytes
	Index     []int     // index sequence for Type.FieldByIndex
	Anonymous bool      // is an embedded field
}

// IsExported reports whether the field is exported.
func (f StructField) IsExported() bool { return f.PkgPath == "" }

// A StructTag is the tag string in a struct field.
type StructTag string

// Get returns the value associated with key in the tag string.
func (tag StructTag) Get(key string) string {
	v, _ := tag.Lookup(key)
	return v
}

// Lookup returns the value associated with key in the tag string.
func (tag StructTag) Lookup(key string) (value string, ok bool) {
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := string(tag[:i])
		tag = tag[i+1:]
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		qvalue := string(tag[:i+1])
		tag = tag[i+1:]
		if key == name {
			value, err := strconv.Unquote(qvalue)
			if err != nil {
				break
			}
			return value, true
		}
	}
	return "", false
}

// SliceHeader is the runtime representation of a slice under gc; goesm has
// no address space, so it is never filled in.
type SliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

// StringHeader is the runtime representation of a string under gc; goesm
// has no address space, so it is never filled in.
type StringHeader struct {
	Data uintptr
	Len  int
}

func (t *rtype) common() *rtype      { return t }
func (t *rtype) Name() string        { return typeName(t) }
func (t *rtype) PkgPath() string     { return typePkgPath(t) }
func (t *rtype) Size() uintptr       { return typeSize(t) }
func (t *rtype) Align() int          { return typeAlign(t) }
func (t *rtype) FieldAlign() int     { return typeAlign(t) }
func (t *rtype) Kind() Kind          { return typeKind(t) }
func (t *rtype) String() string      { return typeString(t) }
func (t *rtype) Comparable() bool    { return typeComparable(t) }
func (t *rtype) IsVariadic() bool    { t.mustBe("IsVariadic", Func); return typeVariadic(t) }
func (t *rtype) NumIn() int          { t.mustBe("NumIn", Func); return typeNumIn(t) }
func (t *rtype) NumOut() int         { t.mustBe("NumOut", Func); return typeNumOut(t) }
func (t *rtype) In(i int) Type       { t.mustBe("In", Func); return toType(typeIn(t, i)) }
func (t *rtype) Out(i int) Type      { t.mustBe("Out", Func); return toType(typeOut(t, i)) }
func (t *rtype) Key() Type           { t.mustBe("Key", Map); return toType(typeKey(t)) }
func (t *rtype) Len() int            { t.mustBe("Len", Array); return typeLen(t) }
func (t *rtype) NumField() int       { t.mustBe("NumField", Struct); return typeNumField(t) }
func (t *rtype) ChanDir() ChanDir    { t.mustBe("ChanDir", Chan); return ChanDir(typeChanDir(t)) }
func (t *rtype) NumMethod() int      { return typeNumMethod(t) }
func (t *rtype) Bits() int           { return t.bits() }
func (t *rtype) CanSeq() bool        { return t.canSeq(1) }
func (t *rtype) CanSeq2() bool       { return t.canSeq(2) }
func (t *rtype) Ins() iter.Seq[Type] { return t.types("Ins", t.NumIn, t.In) }
func (t *rtype) Outs() iter.Seq[Type] {
	return t.types("Outs", t.NumOut, t.Out)
}

func (t *rtype) mustBe(method string, k Kind) {
	if t.Kind() != k {
		panic("reflect: " + method + " of non-" + k.String() + " type " + t.String())
	}
}

func (t *rtype) types(method string, n func() int, at func(int) Type) iter.Seq[Type] {
	t.mustBe(method, Func)
	return func(yield func(Type) bool) {
		for i := range n() {
			if !yield(at(i)) {
				return
			}
		}
	}
}

func (t *rtype) bits() int {
	switch k := t.Kind(); k {
	case Int, Uint, Uintptr, Int64, Uint64, Float64, Complex64:
		return 64
	case Int8, Uint8:
		return 8
	case Int16, Uint16:
		return 16
	case Int32, Uint32, Float32:
		return 32
	case Complex128:
		return 128
	default:
		panic("reflect: Bits of non-arithmetic Type " + t.String())
	}
}

func (t *rtype) canSeq(n int) bool {
	switch t.Kind() {
	case Int8, Int16, Int32, Int64, Int, Uint8, Uint16, Uint32, Uint64, Uint, Uintptr:
		return n == 1
	case Array, Slice, String, Map:
		return true
	case Chan:
		return n == 1 && t.ChanDir()&RecvDir != 0
	case Pointer:
		return t.Elem().Kind() == Array
	case Func:
		if t.NumIn() != 1 || t.NumOut() != 0 {
			return false
		}
		y := t.In(0)
		if y.Kind() != Func || y.NumIn() != n || y.NumOut() != 1 || y.Out(0).Kind() != Bool {
			return false
		}
		return true
	}
	return false
}

func (t *rtype) Elem() Type {
	switch t.Kind() {
	case Array, Chan, Map, Pointer, Slice:
		return toType(typeElem(t))
	}
	panic("reflect: Elem of invalid type " + t.String())
}

func (t *rtype) Field(i int) StructField {
	t.mustBe("Field", Struct)
	if i < 0 || i >= t.NumField() {
		panic("reflect: Field index out of bounds")
	}
	name, pkgPath, ft, tag, embedded, offset := typeField(t, i)
	return StructField{
		Name:      name,
		PkgPath:   pkgPath,
		Type:      toType(ft),
		Tag:       StructTag(tag),
		Offset:    offset,
		Index:     []int{i},
		Anonymous: embedded,
	}
}

func (t *rtype) Fields() iter.Seq[StructField] {
	t.mustBe("Fields", Struct)
	return func(yield func(StructField) bool) {
		for i := range t.NumField() {
			if !yield(t.Field(i)) {
				return
			}
		}
	}
}

func (t *rtype) FieldByIndex(index []int) StructField {
	t.mustBe("FieldByIndex", Struct)
	var f StructField
	var ft Type = t
	for i, x := range index {
		if i > 0 {
			ft = f.Type
			if ft.Kind() == Pointer && ft.Elem().Kind() == Struct {
				ft = ft.Elem()
			}
		}
		f = ft.Field(x)
	}
	f.Index = append([]int(nil), index...)
	return f
}

func (t *rtype) FieldByName(name string) (StructField, bool) {
	t.mustBe("FieldByName", Struct)
	hasEmbeds := false
	if name != "" {
		for i := range t.NumField() {
			f := t.Field(i)
			if f.Name == name {
				return f, true
			}
			if f.Anonymous {
				hasEmbeds = true
			}
		}
	}
	if !hasEmbeds {
		return StructField{}, false
	}
	return t.FieldByNameFunc(func(s string) bool { return s == name })
}

type fieldScan struct {
	typ   *rtype
	index []int
}

// FieldByNameFunc follows the standard library's breadth-first search: a
// name must be unique at the shallowest depth where it appears.
func (t *rtype) FieldByNameFunc(match func(string) bool) (result StructField, ok bool) {
	t.mustBe("FieldByNameFunc", Struct)
	current := []fieldScan{}
	next := []fieldScan{{typ: t}}
	var nextCount map[*rtype]int
	visited := map[*rtype]bool{}
	for len(next) > 0 {
		current, next = next, current[:0]
		count := nextCount
		nextCount = nil
		for _, scan := range current {
			st := scan.typ
			if visited[st] {
				continue
			}
			visited[st] = true
			for i := range st.NumField() {
				f := st.Field(i)
				var ntyp *rtype
				if f.Anonymous {
					ntyp = f.Type.common()
					if ntyp.Kind() == Pointer {
						ntyp = ntyp.Elem().common()
					}
				}
				if match(f.Name) {
					if count[st] > 1 || ok {
						return StructField{}, false
					}
					result = f
					result.Index = append(append([]int(nil), scan.index...), i)
					ok = true
					continue
				}
				if ok || ntyp == nil || ntyp.Kind() != Struct {
					continue
				}
				if nextCount[ntyp] > 0 {
					nextCount[ntyp] = 2
					continue
				}
				if nextCount == nil {
					nextCount = map[*rtype]int{}
				}
				nextCount[ntyp] = 1
				if count[st] > 1 {
					nextCount[ntyp] = 2
				}
				next = append(next, fieldScan{ntyp, append(append([]int(nil), scan.index...), i)})
			}
		}
		if ok {
			break
		}
	}
	return
}

func (t *rtype) Method(i int) Method {
	if i < 0 || i >= t.NumMethod() {
		panic("reflect: Method index out of range")
	}
	name, pkgPath, mt := typeMethod(t, i)
	m := Method{Name: name, PkgPath: pkgPath, Index: i}
	if t.Kind() == Interface {
		m.Type = toType(mt)
		return m
	}
	in := []Type{t}
	for j := range mt.NumIn() {
		in = append(in, mt.In(j))
	}
	var out []Type
	for j := range mt.NumOut() {
		out = append(out, mt.Out(j))
	}
	ft := FuncOf(in, out, mt.IsVariadic()).common()
	m.Type = ft
	m.Func = Value{typ: ft, ptr: methodExpr(t, i), flag: 0}
	return m
}

func (t *rtype) Methods() iter.Seq[Method] {
	return func(yield func(Method) bool) {
		for i := range t.NumMethod() {
			if !yield(t.Method(i)) {
				return
			}
		}
	}
}

func (t *rtype) MethodByName(name string) (Method, bool) {
	for i := range t.NumMethod() {
		n, _, _ := typeMethod(t, i)
		if n == name {
			return t.Method(i), true
		}
	}
	return Method{}, false
}

func (t *rtype) Implements(u Type) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.Implements")
	}
	if u.Kind() != Interface {
		panic("reflect: non-interface type passed to Type.Implements")
	}
	return implements(u.common(), t)
}

func (t *rtype) AssignableTo(u Type) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.AssignableTo")
	}
	uu := u.common()
	return directlyAssignable(uu, t) || implements(uu, t)
}

func (t *rtype) ConvertibleTo(u Type) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.ConvertibleTo")
	}
	return convertible(u.common(), t)
}

func (t *rtype) OverflowComplex(x complex128) bool {
	switch t.Kind() {
	case Complex64:
		return overflowFloat32(real(x)) || overflowFloat32(imag(x))
	case Complex128:
		return false
	}
	panic("reflect: OverflowComplex of non-complex type " + t.String())
}

func (t *rtype) OverflowFloat(x float64) bool {
	switch t.Kind() {
	case Float32:
		return overflowFloat32(x)
	case Float64:
		return false
	}
	panic("reflect: OverflowFloat of non-float type " + t.String())
}

func (t *rtype) OverflowInt(x int64) bool {
	switch t.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		bitSize := uint(t.Size()) * 8
		trunc := (x << (64 - bitSize)) >> (64 - bitSize)
		return x != trunc
	}
	panic("reflect: OverflowInt of non-int type " + t.String())
}

func (t *rtype) OverflowUint(x uint64) bool {
	switch t.Kind() {
	case Uint, Uintptr, Uint8, Uint16, Uint32, Uint64:
		bitSize := uint(t.Size()) * 8
		trunc := (x << (64 - bitSize)) >> (64 - bitSize)
		return x != trunc
	}
	panic("reflect: OverflowUint of non-uint type " + t.String())
}

func overflowFloat32(x float64) bool {
	if x < 0 {
		x = -x
	}
	return 3.40282346638528859811704183484516925440e+38 < x && x <= 1.79769313486231570814527423731704356798070e+308
}

func toType(t *rtype) Type {
	if t == nil {
		return nil
	}
	return t
}

// TypeOf returns the reflection Type that represents the dynamic type of i.
func TypeOf(i any) Type { return toType(ifaceType(i)) }

// TypeFor returns the Type that represents the type argument T.
func TypeFor[T any]() Type { return TypeOf((*T)(nil)).Elem() }

// PtrTo is the old name for PointerTo.
func PtrTo(t Type) Type { return PointerTo(t) }

// PointerTo returns the pointer type with element t.
func PointerTo(t Type) Type { return toType(ptrTo(t.common())) }

// SliceOf returns the slice type with element type t.
func SliceOf(t Type) Type { return toType(sliceOf(t.common())) }

// MapOf returns the map type with the given key and element types.
func MapOf(key, elem Type) Type {
	if !key.Comparable() {
		panic("reflect.MapOf: invalid key type " + key.String())
	}
	return toType(mapOf(key.common(), elem.common()))
}

// ArrayOf returns the array type with the given length and element type.
func ArrayOf(length int, elem Type) Type {
	if length < 0 {
		panic("reflect: negative length passed to ArrayOf")
	}
	return toType(arrayOf(length, elem.common()))
}

// ChanOf returns the channel type with the given direction and element type.
func ChanOf(dir ChanDir, t Type) Type { return toType(chanOf(int(dir), t.common())) }

// FuncOf returns the function type with the given argument and result types.
func FuncOf(in, out []Type, variadic bool) Type {
	if variadic && (len(in) == 0 || in[len(in)-1].Kind() != Slice) {
		panic("reflect.FuncOf: last arg of variadic func must be slice")
	}
	var ins, outs []*rtype
	for _, t := range in {
		ins = append(ins, t.common())
	}
	for _, t := range out {
		outs = append(outs, t.common())
	}
	return toType(funcOf(ins, outs, variadic))
}

// StructOf is not supported by goesm: struct values are instances of
// classes generated at compile time.
func StructOf(fields []StructField) Type {
	panic("reflect.StructOf is not supported by goesm")
}

// VisibleFields returns all the visible fields in t, which must be a struct
// type, in the order of the standard library's implementation.
func VisibleFields(t Type) []StructField {
	if t == nil {
		panic("reflect: VisibleFields(nil)")
	}
	if t.Kind() != Struct {
		panic("reflect.VisibleFields of non-struct type")
	}
	w := &visibleFieldsWalker{
		byName:   make(map[string]int),
		visiting: make(map[Type]bool),
		fields:   make([]StructField, 0, t.NumField()),
		index:    make([]int, 0, 2),
	}
	w.walk(t)
	j := 0
	for i := range w.fields {
		f := &w.fields[i]
		if f.Name == "" {
			continue
		}
		if i != j {
			w.fields[j] = *f
		}
		j++
	}
	return w.fields[:j]
}

type visibleFieldsWalker struct {
	byName   map[string]int
	visiting map[Type]bool
	fields   []StructField
	index    []int
}

func (w *visibleFieldsWalker) walk(t Type) {
	if w.visiting[t] {
		return
	}
	w.visiting[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		w.index = append(w.index, i)
		add := true
		if oldIndex, ok := w.byName[f.Name]; ok {
			old := &w.fields[oldIndex]
			if len(w.index) == len(old.Index) {
				old.Name = ""
				add = false
			} else if len(w.index) < len(old.Index) {
				old.Name = ""
			} else {
				add = false
			}
		}
		if add {
			f.Index = append([]int(nil), w.index...)
			w.byName[f.Name] = len(w.fields)
			w.fields = append(w.fields, f)
		}
		if f.Anonymous {
			if f.Type.Kind() == Pointer {
				f.Type = f.Type.Elem()
			}
			if f.Type.Kind() == Struct {
				w.walk(f.Type)
			}
		}
		w.index = w.index[:len(w.index)-1]
	}
	delete(w.visiting, t)
}

// ---- Value ----

// Value is the reflection interface to a Go value.
type Value struct {
	typ *rtype
	// ptr is the value itself, or a pointer to it when flagAddr is set.
	ptr  unsafe.Pointer
	flag flag
}

type flag uintptr

const (
	flagStickyRO flag = 1 << 5
	flagEmbedRO  flag = 1 << 6
	flagAddr     flag = 1 << 8
	flagMethod   flag = 1 << 9 // ptr is a bound method's function

	flagRO = flagStickyRO | flagEmbedRO
)

func (f flag) ro() flag {
	if f&flagRO != 0 {
		return flagStickyRO
	}
	return 0
}

// A ValueError occurs when a Value method is invoked on a Value that does
// not support it.
type ValueError struct {
	Method string
	Kind   Kind
}

func (e *ValueError) Error() string {
	if e.Kind == 0 {
		return "reflect: call of " + e.Method + " on zero Value"
	}
	return "reflect: call of " + e.Method + " on " + e.Kind.String() + " Value"
}

// ValueOf returns a new Value initialized to the concrete value stored in i.
func ValueOf(i any) Value {
	if i == nil {
		return Value{}
	}
	return Value{typ: ifaceType(i), ptr: ifaceValue(i)}
}

// get returns the value v holds.
func (v Value) get() unsafe.Pointer {
	if v.flag&flagAddr != 0 {
		return load(v.typ, v.ptr)
	}
	return v.ptr
}

func (v Value) mustBe(method string, k Kind) {
	if v.Kind() != k {
		panic(&ValueError{"reflect.Value." + method, v.Kind()})
	}
}

func (v Value) mustBeAssignable(method string) {
	if v.flag&flagRO != 0 {
		panic("reflect: reflect.Value." + method + " using value obtained using unexported field")
	}
	if v.flag&flagAddr == 0 {
		panic("reflect: reflect.Value." + method + " using unaddressable value")
	}
}

func (v Value) mustBeExported(method string) {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value." + method, Invalid})
	}
	if v.flag&flagRO != 0 {
		panic("reflect: reflect.Value." + method + " using value obtained using unexported field")
	}
}

// Kind returns v's Kind. If v is the zero Value, Kind returns Invalid.
func (v Value) Kind() Kind {
	if v.typ == nil {
		return Invalid
	}
	return v.typ.Kind()
}

// IsValid reports whether v represents a value.
func (v Value) IsValid() bool { return v.typ != nil }

// Type returns v's type.
func (v Value) Type() Type {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.Type", Invalid})
	}
	return v.typ
}

// CanAddr reports whether the value's address can be obtained with Addr.
func (v Value) CanAddr() bool { return v.flag&flagAddr != 0 }

// CanSet reports whether the value of v can be changed.
func (v Value) CanSet() bool { return v.flag&(flagAddr|flagRO) == flagAddr }

// CanInterface reports whether Interface can be used without panicking.
func (v Value) CanInterface() bool {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.CanInterface", Invalid})
	}
	return v.flag&flagRO == 0
}

// Interface returns v's current value as an interface{}.
func (v Value) Interface() any { return valueInterface(v, true) }

func valueInterface(v Value, safe bool) any {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.Interface", Invalid})
	}
	if safe && v.flag&flagRO != 0 {
		panic("reflect.Value.Interface: cannot return value obtained from unexported field or method")
	}
	return box(v.typ, v.get())
}

// Addr returns a pointer value representing the address of v.
func (v Value) Addr() Value {
	if v.flag&flagAddr == 0 {
		panic("reflect.Value.Addr of unaddressable value")
	}
	return Value{typ: ptrTo(v.typ), ptr: canonPtr(v.ptr), flag: v.flag.ro()}
}

// UnsafeAddr is not supported: goesm has no address space.
func (v Value) UnsafeAddr() uintptr {
	panic("reflect.Value.UnsafeAddr is not supported by goesm")
}

// InterfaceData is not supported: goesm has no address space.
func (v Value) InterfaceData() [2]uintptr {
	panic("reflect.Value.InterfaceData is not supported by goesm")
}

// Bool returns v's underlying value.
func (v Value) Bool() bool {
	v.mustBe("Bool", Bool)
	return asBool(v.get())
}

// Int returns v's underlying value, as an int64.
func (v Value) Int() int64 {
	switch v.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		return valueInt(v.typ, v.get())
	}
	panic(&ValueError{"reflect.Value.Int", v.Kind()})
}

// Uint returns v's underlying value, as a uint64.
func (v Value) Uint() uint64 {
	switch v.Kind() {
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return valueUint(v.typ, v.get())
	}
	panic(&ValueError{"reflect.Value.Uint", v.Kind()})
}

// Float returns v's underlying value, as a float64.
func (v Value) Float() float64 {
	switch v.Kind() {
	case Float32, Float64:
		return asFloat(v.get())
	}
	panic(&ValueError{"reflect.Value.Float", v.Kind()})
}

// Complex returns v's underlying value, as a complex128.
func (v Value) Complex() complex128 {
	switch v.Kind() {
	case Complex64, Complex128:
		return asComplex(v.get())
	}
	panic(&ValueError{"reflect.Value.Complex", v.Kind()})
}

// OverflowInt reports whether the int64 x cannot be represented by v's type.
func (v Value) OverflowInt(x int64) bool {
	switch v.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		return v.typ.OverflowInt(x)
	}
	panic(&ValueError{"reflect.Value.OverflowInt", v.Kind()})
}

// OverflowUint reports whether the uint64 x cannot be represented by v's type.
func (v Value) OverflowUint(x uint64) bool {
	switch v.Kind() {
	case Uint, Uintptr, Uint8, Uint16, Uint32, Uint64:
		return v.typ.OverflowUint(x)
	}
	panic(&ValueError{"reflect.Value.OverflowUint", v.Kind()})
}

// OverflowFloat reports whether the float64 x cannot be represented by v's type.
func (v Value) OverflowFloat(x float64) bool {
	switch v.Kind() {
	case Float32, Float64:
		return v.typ.OverflowFloat(x)
	}
	panic(&ValueError{"reflect.Value.OverflowFloat", v.Kind()})
}

// OverflowComplex reports whether the complex128 x cannot be represented by v's type.
func (v Value) OverflowComplex(x complex128) bool {
	switch v.Kind() {
	case Complex64, Complex128:
		return v.typ.OverflowComplex(x)
	}
	panic(&ValueError{"reflect.Value.OverflowComplex", v.Kind()})
}

// CanInt reports whether Int can be used without panicking.
func (v Value) CanInt() bool {
	switch v.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		return true
	}
	return false
}

// CanUint reports whether Uint can be used without panicking.
func (v Value) CanUint() bool {
	switch v.Kind() {
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return true
	}
	return false
}

// CanFloat reports whether Float can be used without panicking.
func (v Value) CanFloat() bool {
	switch v.Kind() {
	case Float32, Float64:
		return true
	}
	return false
}

// CanComplex reports whether Complex can be used without panicking.
func (v Value) CanComplex() bool {
	switch v.Kind() {
	case Complex64, Complex128:
		return true
	}
	return false
}

// String returns the string v's underlying value, as a string. Unlike the
// other getters, it does not panic if v's Kind is not String.
func (v Value) String() string {
	if v.Kind() == String {
		return asString(v.get())
	}
	if v.Kind() == Invalid {
		return "<invalid Value>"
	}
	return "<" + v.Type().String() + " Value>"
}

// Bytes returns v's underlying value. It panics if v's underlying value is
// not a slice of bytes or an addressable array of bytes.
func (v Value) Bytes() []byte {
	switch v.Kind() {
	case Slice:
		if v.typ.Elem().Kind() != Uint8 {
			panic("reflect.Value.Bytes of non-byte slice")
		}
		return asBytes(v.get())
	case Array:
		if v.typ.Elem().Kind() != Uint8 {
			panic("reflect.Value.Bytes of non-byte array")
		}
		if !v.CanAddr() {
			panic("reflect.Value.Bytes of unaddressable byte array")
		}
		return asBytes(sliceValue(v.typ, v.ptr, 0, v.Len(), v.Len()))
	}
	panic(&ValueError{"reflect.Value.Bytes", v.Kind()})
}

// Len returns v's length.
func (v Value) Len() int {
	switch k := v.Kind(); k {
	case Array, Chan, Map, Slice, String:
		return length(v.typ, v.get())
	case Pointer:
		if v.typ.Elem().Kind() == Array {
			return v.typ.Elem().Len()
		}
	}
	panic(&ValueError{"reflect.Value.Len", v.Kind()})
}

// Cap returns v's capacity.
func (v Value) Cap() int {
	switch k := v.Kind(); k {
	case Array:
		return v.typ.Len()
	case Chan, Slice:
		return capacity(v.typ, v.get())
	case Pointer:
		if v.typ.Elem().Kind() == Array {
			return v.typ.Elem().Len()
		}
	}
	panic(&ValueError{"reflect.Value.Cap", v.Kind()})
}

// IsNil reports whether its argument v is nil.
func (v Value) IsNil() bool {
	switch k := v.Kind(); k {
	case Chan, Func, Map, Pointer, UnsafePointer, Interface, Slice:
		if v.flag&flagMethod != 0 {
			return false
		}
		return isNil(v.get())
	}
	panic(&ValueError{"reflect.Value.IsNil", v.Kind()})
}

// IsZero reports whether v is the zero value for its type.
func (v Value) IsZero() bool {
	switch v.Kind() {
	case Bool:
		return !v.Bool()
	case Int, Int8, Int16, Int32, Int64:
		return v.Int() == 0
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return v.Uint() == 0
	case Float32, Float64:
		f := v.Float()
		return f == 0 && !signbit(f)
	case Complex64, Complex128:
		c := v.Complex()
		return real(c) == 0 && imag(c) == 0 && !signbit(real(c)) && !signbit(imag(c))
	case Array:
		for i := range v.Len() {
			if !v.Index(i).IsZero() {
				return false
			}
		}
		return true
	case Chan, Func, Interface, Map, Pointer, Slice, UnsafePointer:
		return v.IsNil()
	case String:
		return v.Len() == 0
	case Struct:
		for i := range v.NumField() {
			if !v.Field(i).IsZero() {
				return false
			}
		}
		return true
	}
	panic(&ValueError{"reflect.Value.IsZero", v.Kind()})
}

func signbit(f float64) bool { return f < 0 || 1/f < 0 }

// SetZero sets v to be the zero value of v's type.
func (v Value) SetZero() {
	v.mustBeAssignable("SetZero")
	store(v.typ, v.ptr, zero(v.typ))
}

// Comparable reports whether the value v is comparable.
func (v Value) Comparable() bool {
	switch v.Kind() {
	case Invalid:
		return false
	case Array:
		for i := range v.Len() {
			if !v.Index(i).Comparable() {
				return false
			}
		}
		return true
	case Interface:
		return v.IsNil() || v.Elem().Comparable()
	case Struct:
		for i := range v.NumField() {
			if !v.Field(i).Comparable() {
				return false
			}
		}
		return true
	}
	return v.typ.Comparable()
}

// Equal reports true if v is equal to u.
func (v Value) Equal(u Value) bool {
	if v.Kind() == Interface {
		v = v.Elem()
	}
	if u.Kind() == Interface {
		u = u.Elem()
	}
	if !v.IsValid() || !u.IsValid() {
		return v.IsValid() == u.IsValid()
	}
	if v.Kind() != u.Kind() || v.Type() != u.Type() {
		return false
	}
	if !v.Comparable() {
		panic("reflect.Value.Equal: values of type " + v.Type().String() + " are not comparable")
	}
	return equal(v.typ, v.get(), u.get())
}

// Elem returns the value that the interface v contains or that the pointer
// v points to.
func (v Value) Elem() Value {
	switch k := v.Kind(); k {
	case Interface:
		i := asIface(v.get())
		if i == nil {
			return Value{}
		}
		return Value{typ: ifaceType(i), ptr: ifaceValue(i), flag: v.flag.ro()}
	case Pointer:
		p := v.get()
		if isNil(p) {
			return Value{}
		}
		return Value{typ: typeElem(v.typ), ptr: p, flag: v.flag.ro() | flagAddr}
	}
	panic(&ValueError{"reflect.Value.Elem", v.Kind()})
}

// Indirect returns the value that v points to.
func Indirect(v Value) Value {
	if v.Kind() != Pointer {
		return v
	}
	return v.Elem()
}

// NumField returns the number of fields in the struct v.
func (v Value) NumField() int {
	v.mustBe("NumField", Struct)
	return typeNumField(v.typ)
}

// Field returns the i'th field of the struct v.
func (v Value) Field(i int) Value {
	v.mustBe("Field", Struct)
	if uint(i) >= uint(typeNumField(v.typ)) {
		panic("reflect: Field index out of range")
	}
	name, pkgPath, ft, _, embedded, _ := typeField(v.typ, i)
	fl := v.flag & (flagStickyRO | flagAddr)
	if pkgPath != "" {
		if embedded {
			fl |= flagEmbedRO
		} else {
			fl |= flagStickyRO
		}
	}
	_ = name
	if v.flag&flagAddr != 0 {
		return Value{typ: ft, ptr: fieldAddr(v.typ, v.ptr, i), flag: fl}
	}
	return Value{typ: ft, ptr: fieldValue(v.typ, v.ptr, i), flag: fl}
}

// FieldByIndex returns the nested field corresponding to index.
func (v Value) FieldByIndex(index []int) Value {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	v.mustBe("FieldByIndex", Struct)
	for i, x := range index {
		if i > 0 && v.Kind() == Pointer && v.typ.Elem().Kind() == Struct {
			if v.IsNil() {
				panic("reflect: indirection through nil pointer to embedded struct")
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v
}

// FieldByIndexErr returns the nested field corresponding to index, or an
// error for a nil embedded pointer on the way.
func (v Value) FieldByIndexErr(index []int) (Value, error) {
	if len(index) == 1 {
		return v.Field(index[0]), nil
	}
	v.mustBe("FieldByIndexErr", Struct)
	for i, x := range index {
		if i > 0 && v.Kind() == Pointer && v.typ.Elem().Kind() == Struct {
			if v.IsNil() {
				return Value{}, &nilEmbedError{v.typ.Elem().Name()}
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, nil
}

type nilEmbedError struct{ name string }

func (e *nilEmbedError) Error() string {
	return "reflect: indirection through nil pointer to embedded struct field " + e.name
}

// FieldByName returns the struct field with the given name.
func (v Value) FieldByName(name string) Value {
	if f, ok := v.typ.FieldByName(name); ok {
		return v.FieldByIndex(f.Index)
	}
	return Value{}
}

// FieldByNameFunc returns the struct field with a name that satisfies the
// match function.
func (v Value) FieldByNameFunc(match func(string) bool) Value {
	if f, ok := v.typ.FieldByNameFunc(match); ok {
		return v.FieldByIndex(f.Index)
	}
	return Value{}
}

// Fields returns an iterator over each StructField of v along with its
// Value.
func (v Value) Fields() iter.Seq2[StructField, Value] {
	t := v.Type()
	return func(yield func(StructField, Value) bool) {
		for i := range v.NumField() {
			if !yield(t.Field(i), v.Field(i)) {
				return
			}
		}
	}
}

// Index returns v's i'th element.
func (v Value) Index(i int) Value {
	switch v.Kind() {
	case Array:
		n := v.typ.Len()
		if uint(i) >= uint(n) {
			panic("reflect: array index out of range")
		}
		et := typeElem(v.typ)
		if v.flag&flagAddr != 0 {
			return Value{typ: et, ptr: elemAddr(v.typ, v.ptr, i), flag: v.flag & (flagRO | flagAddr)}
		}
		return Value{typ: et, ptr: elemValue(v.typ, v.ptr, i), flag: v.flag & flagRO}
	case Slice:
		s := v.get()
		if uint(i) >= uint(length(v.typ, s)) {
			panic("reflect: slice index out of range")
		}
		return Value{typ: typeElem(v.typ), ptr: elemAddr(v.typ, s, i), flag: v.flag.ro() | flagAddr}
	case String:
		s := asString(v.get())
		if uint(i) >= uint(len(s)) {
			panic("reflect: string index out of range")
		}
		return Value{typ: uint8Type, ptr: fromUint8(s[i]), flag: v.flag.ro()}
	}
	panic(&ValueError{"reflect.Value.Index", v.Kind()})
}

var uint8Type = TypeOf(uint8(0)).common()

// Slice returns v[i:j].
func (v Value) Slice(i, j int) Value {
	var cap int
	switch k := v.Kind(); k {
	case Array:
		if v.flag&flagAddr == 0 {
			panic("reflect.Value.Slice: slice of unaddressable array")
		}
		cap = v.typ.Len()
	case Slice:
		cap = v.Cap()
	case String:
		s := asString(v.get())
		if i < 0 || j < i || j > len(s) {
			panic("reflect.Value.Slice: string slice index out of bounds")
		}
		return Value{typ: v.typ, ptr: fromString(s[i:j]), flag: v.flag.ro()}
	default:
		panic(&ValueError{"reflect.Value.Slice", v.Kind()})
	}
	if i < 0 || j < i || j > cap {
		panic("reflect.Value.Slice: slice index out of bounds")
	}
	return v.slice3(i, j, cap)
}

// Slice3 is the 3-index form of the slice operation: it returns v[i:j:k].
func (v Value) Slice3(i, j, k int) Value {
	var cap int
	switch v.Kind() {
	case Array:
		if v.flag&flagAddr == 0 {
			panic("reflect.Value.Slice3: slice of unaddressable array")
		}
		cap = v.typ.Len()
	case Slice:
		cap = v.Cap()
	default:
		panic(&ValueError{"reflect.Value.Slice3", v.Kind()})
	}
	if i < 0 || j < i || k < j || k > cap {
		panic("reflect.Value.Slice3: slice index out of bounds")
	}
	return v.slice3(i, j, k)
}

func (v Value) slice3(i, j, k int) Value {
	st := v.typ
	x := v.get()
	if v.Kind() == Array {
		st = sliceOf(typeElem(v.typ))
		x = v.ptr
	}
	return Value{typ: st, ptr: sliceValue(v.typ, x, i, j, k), flag: v.flag.ro()}
}

// Set assigns x to the value v.
func (v Value) Set(x Value) {
	v.mustBeAssignable("Set")
	x.mustBeExported("Set")
	if !directlyAssignable(v.typ, x.typ) && !(v.Kind() == Interface && implements(v.typ, x.typ)) {
		panic("reflect.Set: value of type " + x.typ.String() + " is not assignable to type " + v.typ.String())
	}
	store(v.typ, v.ptr, assignConvert(v.typ, x.typ, x.get()))
}

// SetBool sets v's underlying value.
func (v Value) SetBool(x bool) {
	v.mustBeAssignable("SetBool")
	v.mustBe("SetBool", Bool)
	store(v.typ, v.ptr, fromBool(x))
}

// SetBytes sets v's underlying value.
func (v Value) SetBytes(x []byte) {
	v.mustBeAssignable("SetBytes")
	v.mustBe("SetBytes", Slice)
	if v.typ.Elem().Kind() != Uint8 {
		panic("reflect.Value.SetBytes of non-byte slice")
	}
	store(v.typ, v.ptr, fromBytes(x))
}

// SetInt sets v's underlying value to x.
func (v Value) SetInt(x int64) {
	v.mustBeAssignable("SetInt")
	switch v.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		store(v.typ, v.ptr, makeInt(v.typ, x))
		return
	}
	panic(&ValueError{"reflect.Value.SetInt", v.Kind()})
}

// SetUint sets v's underlying value to x.
func (v Value) SetUint(x uint64) {
	v.mustBeAssignable("SetUint")
	switch v.Kind() {
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		store(v.typ, v.ptr, makeUint(v.typ, x))
		return
	}
	panic(&ValueError{"reflect.Value.SetUint", v.Kind()})
}

// SetFloat sets v's underlying value to x.
func (v Value) SetFloat(x float64) {
	v.mustBeAssignable("SetFloat")
	switch v.Kind() {
	case Float32:
		store(v.typ, v.ptr, fromFloat(float64(float32(x))))
		return
	case Float64:
		store(v.typ, v.ptr, fromFloat(x))
		return
	}
	panic(&ValueError{"reflect.Value.SetFloat", v.Kind()})
}

// SetComplex sets v's underlying value to x.
func (v Value) SetComplex(x complex128) {
	v.mustBeAssignable("SetComplex")
	switch v.Kind() {
	case Complex64:
		store(v.typ, v.ptr, fromComplex(complex128(complex64(x))))
		return
	case Complex128:
		store(v.typ, v.ptr, fromComplex(x))
		return
	}
	panic(&ValueError{"reflect.Value.SetComplex", v.Kind()})
}

// SetString sets v's underlying value to x.
func (v Value) SetString(x string) {
	v.mustBeAssignable("SetString")
	v.mustBe("SetString", String)
	store(v.typ, v.ptr, fromString(x))
}

// SetPointer sets the unsafe.Pointer value v to x.
func (v Value) SetPointer(x unsafe.Pointer) {
	v.mustBeAssignable("SetPointer")
	v.mustBe("SetPointer", UnsafePointer)
	store(v.typ, v.ptr, x)
}

// SetLen sets v's length to n.
func (v Value) SetLen(n int) {
	v.mustBeAssignable("SetLen")
	v.mustBe("SetLen", Slice)
	s := v.get()
	if uint(n) > uint(capacity(v.typ, s)) {
		panic("reflect: slice length out of range in SetLen")
	}
	store(v.typ, v.ptr, sliceValue(v.typ, s, 0, n, capacity(v.typ, s)))
}

// SetCap sets v's capacity to n.
func (v Value) SetCap(n int) {
	v.mustBeAssignable("SetCap")
	v.mustBe("SetCap", Slice)
	s := v.get()
	if n < length(v.typ, s) || n > capacity(v.typ, s) {
		panic("reflect: slice capacity out of range in SetCap")
	}
	store(v.typ, v.ptr, sliceValue(v.typ, s, 0, length(v.typ, s), n))
}

// Grow increases the slice's capacity, if necessary, to guarantee space for
// another n elements.
func (v Value) Grow(n int) {
	v.mustBeAssignable("Grow")
	v.mustBe("Grow", Slice)
	if n < 0 {
		panic("reflect.Value.Grow: negative len")
	}
	s := v.get()
	if length(v.typ, s)+n > capacity(v.typ, s) {
		store(v.typ, v.ptr, grow(v.typ, s, n))
	}
}

// Clear clears the contents of a map or zeros the contents of a slice.
func (v Value) Clear() {
	switch v.Kind() {
	case Slice:
		clearValue(v.typ, v.get())
	case Map:
		clearValue(v.typ, v.get())
	default:
		panic(&ValueError{"reflect.Value.Clear", v.Kind()})
	}
}

// Append appends the values x to a slice s and returns the resulting slice.
func Append(s Value, x ...Value) Value {
	s.mustBe("Append", Slice)
	r := s.get()
	et := typeElem(s.typ)
	for _, e := range x {
		e.mustBeExported("Append")
		if !directlyAssignable(et, e.typ) && !(et.Kind() == Interface && implements(et, e.typ)) {
			panic("reflect.Append: value of type " + e.typ.String() + " is not assignable to type " + et.String())
		}
		r = appendValue(s.typ, r, assignConvert(et, e.typ, e.get()))
	}
	return Value{typ: s.typ, ptr: r, flag: s.flag.ro()}
}

// AppendSlice appends a slice t to a slice s and returns the resulting
// slice.
func AppendSlice(s, t Value) Value {
	s.mustBe("AppendSlice", Slice)
	t.mustBe("AppendSlice", Slice)
	if s.typ.Elem() != t.typ.Elem() {
		panic("reflect.AppendSlice: " + s.typ.String() + " != " + t.typ.String())
	}
	var x []Value
	for i := range t.Len() {
		x = append(x, t.Index(i))
	}
	return Append(s, x...)
}

// Copy copies the contents of src into dst until either dst has been filled
// or src has been exhausted. It returns the number of elements copied.
func Copy(dst, src Value) int {
	dk := dst.Kind()
	if dk != Array && dk != Slice {
		panic(&ValueError{"reflect.Copy", dk})
	}
	if dk == Array && !dst.CanSet() {
		panic("reflect.Copy: unaddressable array value")
	}
	sk := src.Kind()
	if sk != Array && sk != Slice && !(sk == String && dst.typ.Elem().Kind() == Uint8) {
		panic(&ValueError{"reflect.Copy", sk})
	}
	n := dst.Len()
	if m := src.Len(); m < n {
		n = m
	}
	if sk == String {
		s := src.String()
		for i := 0; i < n; i++ {
			dst.Index(i).SetUint(uint64(s[i]))
		}
		return n
	}
	if dst.typ.Elem() != src.typ.Elem() {
		panic("reflect.Copy: " + dst.typ.String() + " != " + src.typ.String())
	}
	// Copy through a snapshot so overlapping ranges behave like copy().
	vals := make([]unsafe.Pointer, n)
	for i := 0; i < n; i++ {
		vals[i] = copyValue(typeElem(src.typ), src.Index(i).get())
	}
	et := typeElem(dst.typ)
	for i := 0; i < n; i++ {
		e := dst.Index(i)
		store(et, e.ptr, vals[i])
	}
	return n
}

// ---- maps ----

// MapIndex returns the value associated with key in the map v.
func (v Value) MapIndex(key Value) Value {
	v.mustBe("MapIndex", Map)
	kt := typeKey(v.typ)
	if !directlyAssignable(kt, key.typ) && !(kt.Kind() == Interface && implements(kt, key.typ)) {
		panic("reflect.Value.MapIndex: value of type " + key.typ.String() + " is not assignable to type " + kt.String())
	}
	e, ok := mapIndex(v.get(), assignConvert(kt, key.typ, key.get()))
	if !ok {
		return Value{}
	}
	return Value{typ: typeElem(v.typ), ptr: e, flag: v.flag.ro() | key.flag.ro()}
}

// MapKeys returns a slice containing all the keys present in the map, in
// unspecified order.
func (v Value) MapKeys() []Value {
	v.mustBe("MapKeys", Map)
	kt := typeKey(v.typ)
	keys := mapKeys(v.get())
	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = Value{typ: kt, ptr: k, flag: v.flag.ro()}
	}
	return out
}

// SetMapIndex sets the element associated with key in the map v to elem. If
// elem is the zero Value, SetMapIndex deletes the key from the map.
func (v Value) SetMapIndex(key, elem Value) {
	v.mustBe("SetMapIndex", Map)
	v.mustBeExported("SetMapIndex")
	key.mustBeExported("SetMapIndex")
	kt := typeKey(v.typ)
	k := assignConvert(kt, key.typ, key.get())
	if elem.typ == nil {
		mapDelete(v.get(), k)
		return
	}
	elem.mustBeExported("SetMapIndex")
	et := typeElem(v.typ)
	if !directlyAssignable(et, elem.typ) && !(et.Kind() == Interface && implements(et, elem.typ)) {
		panic("reflect.Value.SetMapIndex: value of type " + elem.typ.String() + " is not assignable to type " + et.String())
	}
	mapSet(v.get(), k, assignConvert(et, elem.typ, elem.get()))
}

// A MapIter is an iterator for ranging over a map.
type MapIter struct {
	m     Value
	it    unsafe.Pointer // the host iterator over the map's entries, once started
	state int            // 0: before the first Next, 1: at an entry, 2: exhausted
	key   unsafe.Pointer
	val   unsafe.Pointer
}

// MapRange returns a range iterator for a map.
// It panics if v's Kind is not [Map].
func (v Value) MapRange() *MapIter {
	if v.Kind() != Map {
		panic(&ValueError{"reflect.Value.MapRange", v.Kind()})
	}
	return &MapIter{m: v}
}

// Next advances the map iterator and reports whether there is another
// entry. It returns false when iter is exhausted; subsequent
// calls to [MapIter.Key], [MapIter.Value], or [MapIter.Next] will panic.
// Like a range loop, it skips entries deleted during the iteration.
func (iter *MapIter) Next() bool {
	if !iter.m.IsValid() {
		panic("MapIter.Next called on an iterator that does not have an associated map Value")
	}
	if iter.state == 2 {
		panic("MapIter.Next called on exhausted iterator")
	}
	if iter.it == nil {
		iter.it = mapIter(iter.m.get())
	}
	k, e, ok := mapNext(iter.it)
	if !ok {
		iter.key, iter.val = nil, nil
		iter.state = 2
		return false
	}
	iter.key, iter.val = k, e
	iter.state = 1
	return true
}

func (iter *MapIter) valid() bool { return iter.state == 1 }

// Key returns the key of iter's current map entry.
func (iter *MapIter) Key() Value {
	if !iter.valid() {
		panic("MapIter.Key called before Next")
	}
	return Value{typ: typeKey(iter.m.typ), ptr: iter.key, flag: iter.m.flag.ro()}
}

// Value returns the value of iter's current map entry.
func (iter *MapIter) Value() Value {
	if !iter.valid() {
		panic("MapIter.Value called before Next")
	}
	return Value{typ: typeElem(iter.m.typ), ptr: iter.val, flag: iter.m.flag.ro()}
}

// Reset modifies iter to iterate over v.
func (iter *MapIter) Reset(v Value) {
	if v.IsValid() {
		v.mustBe("MapIter.Reset", Map)
	}
	*iter = MapIter{m: v}
}

// SetIterKey assigns to v the key of iter's current map entry.
func (v Value) SetIterKey(iter *MapIter) { v.Set(iter.Key()) }

// SetIterValue assigns to v the value of iter's current map entry.
func (v Value) SetIterValue(iter *MapIter) { v.Set(iter.Value()) }

// ---- methods and calls ----

// NumMethod returns the number of methods in the value's method set.
func (v Value) NumMethod() int {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.NumMethod", Invalid})
	}
	if v.flag&flagMethod != 0 {
		return 0
	}
	return typeNumMethod(v.typ)
}

// Method returns a function value corresponding to v's i'th method.
func (v Value) Method(i int) Value {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.Method", Invalid})
	}
	if v.flag&flagMethod != 0 || uint(i) >= uint(typeNumMethod(v.typ)) {
		panic("reflect: Method index out of range")
	}
	if v.Kind() == Interface && v.IsNil() {
		panic("reflect: Method on nil interface value")
	}
	_, _, mt := typeMethod(v.typ, i)
	return Value{typ: mt, ptr: methodValue(v.typ, v.get(), i), flag: v.flag.ro() | flagMethod}
}

// MethodByName returns a function value corresponding to the method of v
// with the given name.
func (v Value) MethodByName(name string) Value {
	if v.typ == nil {
		panic(&ValueError{"reflect.Value.MethodByName", Invalid})
	}
	if v.flag&flagMethod != 0 {
		panic("reflect: MethodByName of method value")
	}
	for i := range typeNumMethod(v.typ) {
		if n, _, _ := typeMethod(v.typ, i); n == name {
			return v.Method(i)
		}
	}
	return Value{}
}

// Methods returns an iterator over each Method of v's type along with the
// corresponding method value.
func (v Value) Methods() iter.Seq2[Method, Value] {
	return func(yield func(Method, Value) bool) {
		rtype := v.Type()
		for i := range v.NumMethod() {
			if !yield(rtype.Method(i), v.Method(i)) {
				return
			}
		}
	}
}

// Call calls the function v with the input arguments in.
func (v Value) Call(in []Value) []Value {
	v.mustBe("Call", Func)
	v.mustBeExported("Call")
	return v.call("Call", in, false)
}

// CallSlice calls the variadic function v with the input arguments in,
// assigning the slice in[len(in)-1] to v's final variadic argument.
func (v Value) CallSlice(in []Value) []Value {
	v.mustBe("CallSlice", Func)
	v.mustBeExported("CallSlice")
	return v.call("CallSlice", in, true)
}

func (v Value) call(op string, in []Value, isSlice bool) []Value {
	t := v.typ
	fn := v.get()
	if isNil(fn) {
		panic("reflect: call of nil function")
	}
	n := t.NumIn()
	isVariadic := t.IsVariadic()
	if isSlice {
		if !isVariadic {
			panic("reflect: CallSlice of non-variadic function")
		}
		if len(in) < n {
			panic("reflect: CallSlice with too few input arguments")
		}
		if len(in) > n {
			panic("reflect: CallSlice with too many input arguments")
		}
	} else {
		if isVariadic {
			n--
		}
		if len(in) < n {
			panic("reflect: Call with too few input arguments")
		}
		if !isVariadic && len(in) > n {
			panic("reflect: Call with too many input arguments")
		}
	}
	for _, x := range in {
		if x.Kind() == Invalid {
			panic("reflect: " + op + " using zero Value argument")
		}
	}
	for i := 0; i < n; i++ {
		if xt, targ := in[i].Type(), t.In(i); !xt.AssignableTo(targ) {
			panic("reflect: " + op + " using " + xt.String() + " as type " + targ.String())
		}
	}
	if !isSlice && isVariadic {
		m := len(in) - n
		slice := MakeSlice(t.In(n), m, m)
		elem := t.In(n).Elem()
		for i := 0; i < m; i++ {
			x := in[n+i]
			if xt := x.Type(); !xt.AssignableTo(elem) {
				panic("reflect: cannot use " + xt.String() + " as type " + elem.String() + " in " + op)
			}
			slice.Index(i).Set(x)
		}
		origIn := in
		in = make([]Value, n+1)
		copy(in[:n], origIn)
		in[n] = slice
	}
	args := make([]unsafe.Pointer, len(in))
	for i, x := range in {
		pt := t.In(i).common()
		args[i] = assignConvert(pt, x.typ, x.get())
	}
	results := callFunc(t, fn, args)
	out := make([]Value, t.NumOut())
	for i := range out {
		out[i] = Value{typ: t.Out(i).common(), ptr: results[i]}
	}
	return out
}

// MakeFunc returns a new function of the given Type that wraps the function
// fn.
func MakeFunc(typ Type, fn func(args []Value) (results []Value)) Value {
	if typ.Kind() != Func {
		panic("reflect: call of MakeFunc with non-Func type")
	}
	t := typ.common()
	impl := func(args []unsafe.Pointer) []unsafe.Pointer {
		in := make([]Value, len(args))
		for i, a := range args {
			in[i] = Value{typ: t.In(i).common(), ptr: a}
		}
		out := fn(in)
		if len(out) != t.NumOut() {
			panic("reflect: wrong return count from function created by MakeFunc")
		}
		res := make([]unsafe.Pointer, len(out))
		for i, r := range out {
			if r.typ == nil {
				panic("reflect: function created by MakeFunc using closure returned zero Value")
			}
			if r.flag&flagRO != 0 {
				panic("reflect: function created by MakeFunc using closure returned value obtained from unexported field")
			}
			ot := t.Out(i).common()
			if !directlyAssignable(ot, r.typ) && !(ot.Kind() == Interface && implements(ot, r.typ)) {
				panic("reflect: function created by MakeFunc using closure returned wrong type: have " + r.typ.String() + " for " + ot.String())
			}
			res[i] = assignConvert(ot, r.typ, r.get())
		}
		return res
	}
	return Value{typ: t, ptr: makeFunc(t, impl)}
}

// ---- conversions and construction ----

// Convert returns the value v converted to type t.
func (v Value) Convert(t Type) Value {
	if v.flag&flagMethod != 0 {
		panic("reflect.Value.Convert: method values are not supported")
	}
	tt := t.common()
	if !convertible(tt, v.typ) {
		panic("reflect.Value.Convert: value of type " + v.typ.String() + " cannot be converted to type " + t.String())
	}
	if tt.Kind() == Pointer && tt.Elem().Kind() == Array && v.Kind() == Slice && v.Len() < tt.Elem().Len() {
		panic("reflect: cannot convert slice with length " + strconv.Itoa(v.Len()) + " to pointer to array with length " + strconv.Itoa(tt.Elem().Len()))
	}
	if tt.Kind() == Array && v.Kind() == Slice && v.Len() < tt.Len() {
		panic("reflect: cannot convert slice with length " + strconv.Itoa(v.Len()) + " to array with length " + strconv.Itoa(tt.Len()))
	}
	return Value{typ: tt, ptr: convertValue(tt, v.typ, v.get()), flag: v.flag.ro()}
}

// CanConvert reports whether the value v can be converted to type t.
func (v Value) CanConvert(t Type) bool {
	vt := v.Type()
	if !vt.ConvertibleTo(t) {
		return false
	}
	switch {
	case vt.Kind() == Slice && t.Kind() == Array:
		if t.Len() > v.Len() {
			return false
		}
	case vt.Kind() == Slice && t.Kind() == Pointer && t.Elem().Kind() == Array:
		n := t.Elem().Len()
		if n > v.Len() {
			return false
		}
	}
	return true
}

// New returns a Value representing a pointer to a new zero value for the
// specified type.
func New(typ Type) Value {
	if typ == nil {
		panic("reflect: New(nil)")
	}
	t := typ.common()
	return Value{typ: ptrTo(t), ptr: newPtr(t)}
}

// NewAt returns a Value representing a pointer to a value of the specified
// type, using p as that pointer. goesm resolves p, which may point into the
// middle of an aggregate (runtime/src/unsafe.ts), to a pointer to typ.
func NewAt(typ Type, p unsafe.Pointer) Value {
	t := typ.common()
	return Value{typ: ptrTo(t), ptr: pointerAt(p, t)}
}

// SliceAt is not supported: goesm has no address space.
func SliceAt(typ Type, p unsafe.Pointer, n int) Value {
	panic("reflect.SliceAt is not supported by goesm")
}

// Zero returns a Value representing the zero value for the specified type.
func Zero(typ Type) Value {
	if typ == nil {
		panic("reflect: Zero(nil)")
	}
	t := typ.common()
	return Value{typ: t, ptr: zero(t)}
}

// MakeSlice creates a new zero-initialized slice value for the specified
// slice type, length, and capacity.
func MakeSlice(typ Type, len, cap int) Value {
	if typ.Kind() != Slice {
		panic("reflect.MakeSlice of non-slice type")
	}
	if len < 0 {
		panic("reflect.MakeSlice: negative len")
	}
	if cap < 0 {
		panic("reflect.MakeSlice: negative cap")
	}
	if len > cap {
		panic("reflect.MakeSlice: len > cap")
	}
	t := typ.common()
	return Value{typ: t, ptr: makeSlice(t, len, cap)}
}

// MakeMap creates a new map with the specified type.
func MakeMap(typ Type) Value { return MakeMapWithSize(typ, 0) }

// MakeMapWithSize creates a new map with the specified type and initial
// space for approximately n elements.
func MakeMapWithSize(typ Type, n int) Value {
	if typ.Kind() != Map {
		panic("reflect.MakeMapWithSize of non-map type")
	}
	t := typ.common()
	return Value{typ: t, ptr: makeMap(t)}
}

// MakeChan creates a new channel with the specified type and buffer size.
func MakeChan(typ Type, buffer int) Value {
	if typ.Kind() != Chan {
		panic("reflect.MakeChan of non-chan type")
	}
	if buffer < 0 {
		panic("reflect.MakeChan: negative buffer size")
	}
	if typ.ChanDir() != BothDir {
		panic("reflect.MakeChan: unidirectional channel type")
	}
	t := typ.common()
	return Value{typ: t, ptr: makeChan(t, buffer)}
}

// ---- pointers ----

// Pointer returns v's value as a uintptr: a number identifying the pointer,
// map, channel, function or slice array (goesm has no addresses).
func (v Value) Pointer() uintptr {
	switch v.Kind() {
	case Pointer, Chan, Map, UnsafePointer, Func, Slice:
		if v.flag&flagMethod != 0 {
			return methodValueCode
		}
		return pointerID(v.typ, v.get())
	}
	panic(&ValueError{"reflect.Value.Pointer", v.Kind()})
}

// methodValueCode stands for the code pointer every method value shares.
const methodValueCode uintptr = 1

// UnsafePointer returns v's value as an unsafe.Pointer.
func (v Value) UnsafePointer() unsafe.Pointer {
	switch v.Kind() {
	case Pointer, Chan, Map, UnsafePointer, Func, Slice:
		return unsafePointer(v.typ, v.get())
	}
	panic(&ValueError{"reflect.Value.UnsafePointer", v.Kind()})
}

// ---- channels ----

// Send sends x on the channel v. goesm supports it only when it does not
// block.
func (v Value) Send(x Value) {
	if !v.TrySend(x) {
		panic("reflect.Value.Send would block (blocking reflect channel operations are not supported by goesm)")
	}
}

// TrySend attempts to send x on the channel v but will not block.
func (v Value) TrySend(x Value) bool {
	v.mustBe("TrySend", Chan)
	v.mustBeExported("TrySend")
	if ChanDir(typeChanDir(v.typ))&SendDir == 0 {
		panic("reflect: send on recv-only channel")
	}
	x.mustBeExported("TrySend")
	et := typeElem(v.typ)
	return trySend(v.get(), assignConvert(et, x.typ, x.get()))
}

// Recv receives and returns a value from the channel v. goesm supports it
// only when it does not block.
func (v Value) Recv() (x Value, ok bool) {
	x, ok, selected := v.tryRecv()
	if !selected {
		panic("reflect.Value.Recv would block (blocking reflect channel operations are not supported by goesm)")
	}
	return x, ok
}

// TryRecv attempts to receive a value from the channel v but will not block.
func (v Value) TryRecv() (x Value, ok bool) {
	x, ok, _ = v.tryRecv()
	return x, ok
}

func (v Value) tryRecv() (Value, bool, bool) {
	v.mustBe("TryRecv", Chan)
	v.mustBeExported("TryRecv")
	if ChanDir(typeChanDir(v.typ))&RecvDir == 0 {
		panic("reflect: recv on send-only channel")
	}
	x, ok, selected := tryRecv(v.get())
	if !selected {
		return Value{}, false, false
	}
	return Value{typ: typeElem(v.typ), ptr: x}, ok, true
}

// Close closes the channel v.
func (v Value) Close() {
	v.mustBe("Close", Chan)
	v.mustBeExported("Close")
	if ChanDir(typeChanDir(v.typ))&SendDir == 0 {
		panic("reflect: close of receive-only channel")
	}
	chanClose(v.get())
}

// A SelectDir describes the communication direction of a select case.
type SelectDir int

const (
	_ SelectDir = iota
	SelectSend
	SelectRecv
	SelectDefault
)

// A SelectCase describes a single case in a select operation.
type SelectCase struct {
	Dir  SelectDir // direction of case
	Chan Value     // channel to use (for send or receive)
	Send Value     // value to send (for send)
}

// Select executes a select operation described by the list of cases. goesm
// supports it only when a case is ready or there is a default case.
func Select(cases []SelectCase) (chosen int, recv Value, recvOK bool) {
	dflt := -1
	for i, c := range cases {
		switch c.Dir {
		case SelectDefault:
			if dflt >= 0 {
				panic("reflect.Select: multiple default cases")
			}
			dflt = i
		case SelectSend:
			if c.Chan.IsValid() && !c.Chan.IsNil() && c.Chan.TrySend(c.Send) {
				return i, Value{}, false
			}
		case SelectRecv:
			if c.Chan.IsValid() && !c.Chan.IsNil() {
				if x, ok, selected := c.Chan.tryRecv(); selected {
					return i, x, ok
				}
			}
		default:
			panic("reflect.Select: invalid Dir")
		}
	}
	if dflt >= 0 {
		return dflt, Value{}, false
	}
	panic("reflect.Select would block (blocking reflect channel operations are not supported by goesm)")
}

// ---- iteration ----

// Seq returns an iter.Seq[Value] that loops over the elements of v.
func (v Value) Seq() iter.Seq[Value] {
	switch v.Kind() {
	case Int, Int8, Int16, Int32, Int64:
		return func(yield func(Value) bool) {
			for i := range v.Int() {
				if !yield(ValueOf(i).Convert(v.Type())) {
					return
				}
			}
		}
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return func(yield func(Value) bool) {
			for i := range v.Uint() {
				if !yield(ValueOf(i).Convert(v.Type())) {
					return
				}
			}
		}
	case Pointer:
		if v.Elem().Kind() != Array {
			break
		}
		return func(yield func(Value) bool) {
			v = v.Elem()
			for i := range v.Len() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case Array, Slice:
		return func(yield func(Value) bool) {
			for i := range v.Len() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case String:
		return func(yield func(Value) bool) {
			for i := range v.String() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case Map:
		return func(yield func(Value) bool) {
			i := v.MapRange()
			for i.Next() {
				if !yield(i.Key()) {
					return
				}
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq[Value]")
}

// Seq2 returns an iter.Seq2[Value, Value] that loops over the elements of v.
func (v Value) Seq2() iter.Seq2[Value, Value] {
	switch v.Kind() {
	case Pointer:
		if v.Elem().Kind() != Array {
			break
		}
		return func(yield func(Value, Value) bool) {
			v = v.Elem()
			for i := range v.Len() {
				if !yield(ValueOf(i), v.Index(i)) {
					return
				}
			}
		}
	case Array, Slice:
		return func(yield func(Value, Value) bool) {
			for i := range v.Len() {
				if !yield(ValueOf(i), v.Index(i)) {
					return
				}
			}
		}
	case String:
		return func(yield func(Value, Value) bool) {
			for i, r := range v.String() {
				if !yield(ValueOf(i), ValueOf(r)) {
					return
				}
			}
		}
	case Map:
		return func(yield func(Value, Value) bool) {
			i := v.MapRange()
			for i.Next() {
				if !yield(i.Key(), i.Value()) {
					return
				}
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq2[Value, Value]")
}

// TypeAssert is semantically equivalent to v2, ok := v.Interface().(T).
func TypeAssert[T any](v Value) (T, bool) {
	if v.typ == nil {
		panic(&ValueError{"reflect.TypeAssert", Invalid})
	}
	if v.flag&flagRO != 0 {
		panic("reflect.TypeAssert: cannot return value obtained from unexported field or method")
	}
	x, ok := valueInterface(v, false).(T)
	return x, ok
}

// Swapper returns a function that swaps the elements in the provided slice.
func Swapper(slice any) func(i, j int) {
	v := ValueOf(slice)
	if v.Kind() != Slice {
		panic(&ValueError{Method: "Swapper", Kind: v.Kind()})
	}
	return swapper(v.typ, v.ptr)
}

// ---- DeepEqual ----

type visit struct {
	a1  unsafe.Pointer
	a2  unsafe.Pointer
	typ Type
}

// DeepEqual reports whether x and y are "deeply equal", as defined by the
// standard library's reflect.DeepEqual.
func DeepEqual(x, y any) bool {
	if x == nil || y == nil {
		return x == y
	}
	v1 := ValueOf(x)
	v2 := ValueOf(y)
	if v1.Type() != v2.Type() {
		return false
	}
	return deepValueEqual(v1, v2, make(map[visit]bool))
}

func deepValueEqual(v1, v2 Value, visited map[visit]bool) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}
	switch v1.Kind() {
	case Pointer, Map, Slice, Interface:
		if !v1.IsNil() && !v2.IsNil() {
			a1, a2 := v1.get(), v2.get()
			v := visit{a1, a2, v1.Type()}
			if visited[v] {
				return true
			}
			visited[v] = true
		}
	}
	switch v1.Kind() {
	case Array:
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if sameSlice(v1.get(), v2.get()) {
			return true
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case Pointer:
		if v1.get() == v2.get() {
			return true
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case Struct:
		for i, n := 0, v1.NumField(); i < n; i++ {
			if !deepValueEqual(v1.Field(i), v2.Field(i), visited) {
				return false
			}
		}
		return true
	case Map:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.get() == v2.get() {
			return true
		}
		iter := v1.MapRange()
		for iter.Next() {
			val1 := iter.Value()
			val2 := v2.MapIndex(iter.Key())
			if !val1.IsValid() || !val2.IsValid() || !deepValueEqual(val1, val2, visited) {
				return false
			}
		}
		return true
	case Func:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		return false
	case Int, Int8, Int16, Int32, Int64:
		return v1.Int() == v2.Int()
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return v1.Uint() == v2.Uint()
	case String:
		return v1.String() == v2.String()
	case Bool:
		return v1.Bool() == v2.Bool()
	case Float32, Float64:
		return v1.Float() == v2.Float()
	case Complex64, Complex128:
		return v1.Complex() == v2.Complex()
	}
	return equal(v1.typ, v1.get(), v2.get())
}

// isLetter and isValidFieldName follow the standard library (used by
// packages that validate names the way reflect does).
func isLetter(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch >= utf8.RuneSelf && unicode.IsLetter(ch)
}

func isValidFieldName(fieldName string) bool {
	for i, c := range fieldName {
		if i == 0 && !isLetter(c) {
			return false
		}
		if !(isLetter(c) || unicode.IsDigit(c)) {
			return false
		}
	}
	return len(fieldName) > 0
}

var _ = isValidFieldName

// Natives (runtime/src/natives.ts).

func ifaceType(i any) *rtype
func ifaceValue(i any) unsafe.Pointer
func asIface(x unsafe.Pointer) any
func box(t *rtype, x unsafe.Pointer) any

func typeName(t *rtype) string
func typePkgPath(t *rtype) string
func typeSize(t *rtype) uintptr
func typeAlign(t *rtype) int
func typeKind(t *rtype) Kind
func typeString(t *rtype) string
func typeComparable(t *rtype) bool
func typeElem(t *rtype) *rtype
func typeKey(t *rtype) *rtype
func typeLen(t *rtype) int
func typeChanDir(t *rtype) int
func typeNumField(t *rtype) int
func typeField(t *rtype, i int) (name, pkgPath string, typ *rtype, tag string, embedded bool, offset uintptr)
func typeNumIn(t *rtype) int
func typeIn(t *rtype, i int) *rtype
func typeNumOut(t *rtype) int
func typeOut(t *rtype, i int) *rtype
func typeVariadic(t *rtype) bool
func typeNumMethod(t *rtype) int
func typeMethod(t *rtype, i int) (name, pkgPath string, typ *rtype)
func implements(iface, t *rtype) bool
func directlyAssignable(dst, src *rtype) bool
func convertible(dst, src *rtype) bool
func ptrTo(t *rtype) *rtype
func sliceOf(t *rtype) *rtype
func mapOf(key, elem *rtype) *rtype
func arrayOf(n int, elem *rtype) *rtype
func chanOf(dir int, elem *rtype) *rtype
func funcOf(in, out []*rtype, variadic bool) *rtype

func load(t *rtype, p unsafe.Pointer) unsafe.Pointer
func store(t *rtype, p, x unsafe.Pointer)
func zero(t *rtype) unsafe.Pointer
func newPtr(t *rtype) unsafe.Pointer
func copyValue(t *rtype, x unsafe.Pointer) unsafe.Pointer
func assignConvert(dst, src *rtype, x unsafe.Pointer) unsafe.Pointer
func convertValue(dst, src *rtype, x unsafe.Pointer) unsafe.Pointer
func equal(t *rtype, x, y unsafe.Pointer) bool
func isNil(x unsafe.Pointer) bool
func length(t *rtype, x unsafe.Pointer) int
func capacity(t *rtype, x unsafe.Pointer) int
func fieldAddr(t *rtype, p unsafe.Pointer, i int) unsafe.Pointer

// canonPtr returns the Go pointer value equal to p, a pointer fieldAddr or
// elemAddr made (those leave out the cache that gives &x its identity).
func canonPtr(p unsafe.Pointer) unsafe.Pointer
func fieldValue(t *rtype, x unsafe.Pointer, i int) unsafe.Pointer
func elemAddr(t *rtype, x unsafe.Pointer, i int) unsafe.Pointer
func elemValue(t *rtype, x unsafe.Pointer, i int) unsafe.Pointer
func sliceValue(t *rtype, x unsafe.Pointer, i, j, k int) unsafe.Pointer
func sameSlice(x, y unsafe.Pointer) bool
func makeSlice(t *rtype, len, cap int) unsafe.Pointer
func appendValue(t *rtype, s, x unsafe.Pointer) unsafe.Pointer
func grow(t *rtype, s unsafe.Pointer, n int) unsafe.Pointer
func clearValue(t *rtype, x unsafe.Pointer)
func swapper(t *rtype, s unsafe.Pointer) func(i, j int)

func makeMap(t *rtype) unsafe.Pointer
func mapIndex(m, k unsafe.Pointer) (unsafe.Pointer, bool)
func mapSet(m, k, x unsafe.Pointer)
func mapDelete(m, k unsafe.Pointer)
func mapKeys(m unsafe.Pointer) []unsafe.Pointer

// mapIter starts an iteration over the entries of map m (nil for a nil map);
// mapNext returns the next live entry, as a range loop would.
func mapIter(m unsafe.Pointer) unsafe.Pointer

func mapNext(it unsafe.Pointer) (k, e unsafe.Pointer, ok bool)

func makeChan(t *rtype, n int) unsafe.Pointer
func trySend(ch, x unsafe.Pointer) bool
func tryRecv(ch unsafe.Pointer) (x unsafe.Pointer, ok, selected bool)
func chanClose(ch unsafe.Pointer)

func methodValue(t *rtype, x unsafe.Pointer, i int) unsafe.Pointer
func methodExpr(t *rtype, i int) unsafe.Pointer
func callFunc(t *rtype, fn unsafe.Pointer, args []unsafe.Pointer) []unsafe.Pointer
func makeFunc(t *rtype, impl func([]unsafe.Pointer) []unsafe.Pointer) unsafe.Pointer
func pointerID(t *rtype, x unsafe.Pointer) uintptr
func unsafePointer(t *rtype, x unsafe.Pointer) unsafe.Pointer
func pointerAt(p unsafe.Pointer, t *rtype) unsafe.Pointer

func asBool(x unsafe.Pointer) bool
func asFloat(x unsafe.Pointer) float64
func asComplex(x unsafe.Pointer) complex128
func asString(x unsafe.Pointer) string
func asBytes(x unsafe.Pointer) []byte
func valueInt(t *rtype, x unsafe.Pointer) int64
func valueUint(t *rtype, x unsafe.Pointer) uint64
func makeInt(t *rtype, x int64) unsafe.Pointer
func makeUint(t *rtype, x uint64) unsafe.Pointer
func fromBool(x bool) unsafe.Pointer
func fromFloat(x float64) unsafe.Pointer
func fromComplex(x complex128) unsafe.Pointer
func fromString(x string) unsafe.Pointer
func fromBytes(x []byte) unsafe.Pointer
func fromUint8(x uint8) unsafe.Pointer
