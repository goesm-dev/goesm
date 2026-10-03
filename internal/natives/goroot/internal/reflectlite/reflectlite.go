//go:build goesm

// Package reflectlite is goesm's replacement for the standard library's
// lightweight reflection package (used by errors, sort and context). A Type
// is a @goesm/runtime type descriptor; a Value holds a JS value, or for an
// addressable Value a pointer to it. The descriptor and value operations
// are natives (runtime/src/natives.ts); everything else is ordinary Go.
package reflectlite

import "unsafe"

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
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return kindNames[0]
}

// Type is the representation of a Go type.
type Type interface {
	Name() string
	PkgPath() string
	Size() uintptr
	Kind() Kind
	Implements(u Type) bool
	AssignableTo(u Type) bool
	Comparable() bool
	String() string
	Elem() Type
	common() *rtype
}

// rtype is a runtime type descriptor; Go code never looks inside it.
type rtype struct {
	_ [0]func()
}

func (t *rtype) common() *rtype   { return t }
func (t *rtype) Name() string     { return typeName(t) }
func (t *rtype) PkgPath() string  { return typePkgPath(t) }
func (t *rtype) Size() uintptr    { return typeSize(t) }
func (t *rtype) Kind() Kind       { return typeKind(t) }
func (t *rtype) String() string   { return typeString(t) }
func (t *rtype) Comparable() bool { return typeComparable(t) }

func (t *rtype) Elem() Type {
	switch t.Kind() {
	case Array, Chan, Map, Pointer, Slice:
		return toType(typeElem(t))
	}
	panic("reflect: Elem of invalid type " + t.String())
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

func toType(t *rtype) Type {
	if t == nil {
		return nil
	}
	return t
}

// TypeOf returns the reflection Type that represents the dynamic type of i.
func TypeOf(i any) Type {
	return toType(ifaceType(i))
}

// Value is the reflection interface to a Go value.
type Value struct {
	typ *rtype
	// ptr is the value itself, or a pointer to it when flagAddr is set.
	ptr  unsafe.Pointer
	flag flag
}

type flag uintptr

const (
	flagRO   flag = 1 << 5
	flagAddr flag = 1 << 8
)

// ValueOf returns a new Value initialized to the concrete value stored in i.
func ValueOf(i any) Value {
	if i == nil {
		return Value{}
	}
	return Value{typ: ifaceType(i), ptr: ifaceValue(i)}
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

func (v Value) load() unsafe.Pointer {
	if v.flag&flagAddr != 0 {
		return load(v.typ, v.ptr)
	}
	return v.ptr
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
		panic(&ValueError{"reflectlite.Value.Type", Invalid})
	}
	return v.typ
}

// CanSet reports whether the value of v can be changed.
func (v Value) CanSet() bool { return v.flag&(flagAddr|flagRO) == flagAddr }

// Elem returns the value that the interface v contains or that the pointer
// v points to.
func (v Value) Elem() Value {
	switch k := v.Kind(); k {
	case Interface:
		i := asIface(v.load())
		if i == nil {
			return Value{}
		}
		return Value{typ: ifaceType(i), ptr: ifaceValue(i), flag: v.flag & flagRO}
	case Pointer:
		p := v.load()
		if p == nil {
			return Value{}
		}
		return Value{typ: typeElem(v.typ), ptr: p, flag: v.flag&flagRO | flagAddr}
	default:
		panic(&ValueError{"reflectlite.Value.Elem", k})
	}
}

// IsNil reports whether its argument v is nil.
func (v Value) IsNil() bool {
	switch k := v.Kind(); k {
	case Chan, Func, Map, Pointer, UnsafePointer, Interface, Slice:
		return v.load() == nil
	default:
		panic(&ValueError{"reflectlite.Value.IsNil", k})
	}
}

// Len returns v's length.
func (v Value) Len() int {
	switch k := v.Kind(); k {
	case Array, Chan, Map, Slice, String:
		return length(v.typ, v.load())
	default:
		panic(&ValueError{"reflect.Value.Len", k})
	}
}

// Set assigns x to the value v.
func (v Value) Set(x Value) {
	if v.flag&flagAddr == 0 {
		panic("reflect: reflectlite.Value.Set using unaddressable value")
	}
	if v.flag&flagRO != 0 {
		panic("reflect: reflectlite.Value.Set using value obtained using unexported field")
	}
	if x.typ == nil {
		panic(&ValueError{"reflectlite.Value.Set", Invalid})
	}
	if !directlyAssignable(v.typ, x.typ) && !implements(v.typ, x.typ) {
		panic("reflect: reflectlite.Set: value of type " + x.typ.String() + " is not assignable to type " + v.typ.String())
	}
	store(v.typ, v.ptr, convert(v.typ, x.typ, x.load()))
}

// Swapper returns a function that swaps the elements in the provided slice.
func Swapper(slice any) func(i, j int) {
	v := ValueOf(slice)
	if v.Kind() != Slice {
		panic(&ValueError{Method: "Swapper", Kind: v.Kind()})
	}
	return swapper(v.typ, v.ptr)
}

// Natives (runtime/src/natives.ts).

func ifaceType(i any) *rtype
func ifaceValue(i any) unsafe.Pointer
func asIface(x unsafe.Pointer) any
func typeName(t *rtype) string
func typePkgPath(t *rtype) string
func typeSize(t *rtype) uintptr
func typeKind(t *rtype) Kind
func typeString(t *rtype) string
func typeComparable(t *rtype) bool
func typeElem(t *rtype) *rtype
func implements(iface, t *rtype) bool
func directlyAssignable(dst, src *rtype) bool
func load(t *rtype, p unsafe.Pointer) unsafe.Pointer
func store(t *rtype, p, x unsafe.Pointer)
func convert(dst, src *rtype, x unsafe.Pointer) unsafe.Pointer
func length(t *rtype, x unsafe.Pointer) int
func swapper(t *rtype, s unsafe.Pointer) func(i, j int)
