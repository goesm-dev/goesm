//go:build goesm

// Package js is goesm's replacement of syscall/js. Under gc, a Value is a
// handle (a NaN-boxed index into a table of JavaScript values kept by
// wasm_exec.js). goesm runs as JavaScript, so a Value holds the JavaScript
// value itself and the package is a thin layer over runtime natives. The
// exported API is the one of the standard syscall/js.
package js

import "unsafe"

// ref holds a JavaScript value as is, except that Go's zero value (nil)
// stands for undefined, so that the zero Value is undefined as in Go, and
// JavaScript null is a sentinel of the runtime.
type ref unsafe.Pointer

// Value represents a JavaScript value. The zero value is the JavaScript
// value "undefined". Values can be checked for equality with the Equal
// method.
type Value struct {
	_   [0]func() // uncomparable; to make == not compile
	ref ref
}

// Error wraps a JavaScript error.
type Error struct {
	// Value is the underlying JavaScript error value.
	Value
}

// Error implements the error interface.
func (e Error) Error() string {
	return "JavaScript error: " + e.Get("message").String()
}

func makeValue(r ref) Value { return Value{ref: r} }

// Equal reports whether v and w are equal according to JavaScript's ===
// operator.
func (v Value) Equal(w Value) bool { return valueEqual(v.ref, w.ref) }

// Undefined returns the JavaScript value "undefined".
func Undefined() Value { return Value{} }

// IsUndefined reports whether v is the JavaScript value "undefined".
func (v Value) IsUndefined() bool { return v.ref == nil }

// Null returns the JavaScript value "null".
func Null() Value { return makeValue(nullRef()) }

// IsNull reports whether v is the JavaScript value "null".
func (v Value) IsNull() bool { return v.ref == nullRef() }

// IsNaN reports whether v is the JavaScript value "NaN".
func (v Value) IsNaN() bool { return valueIsNaN(v.ref) }

// Global returns the JavaScript global object, usually "window" or
// "global". Its "fs" property is goesm's file system (node:fs where the host
// has it, the console otherwise) with the callback API package syscall uses,
// whatever the global object holds; "process" falls back to a minimal
// implementation where the host has none (in browsers). Neither changes the
// real global object.
func Global() Value { return makeValue(globalRef()) }

// ValueOf returns x as a JavaScript value:
//
//	| Go                     | JavaScript             |
//	| ---------------------- | ---------------------- |
//	| js.Value               | [its value]            |
//	| js.Func                | function               |
//	| nil                    | null                   |
//	| bool                   | boolean                |
//	| integers and floats    | number                 |
//	| string                 | string                 |
//	| []interface{}          | new array              |
//	| map[string]interface{} | new object             |
//
// Panics if x is not one of the expected types.
func ValueOf(x any) Value {
	switch x := x.(type) {
	case Value:
		return x
	case Func:
		return x.Value
	case nil:
		return Null()
	case bool:
		return makeValue(boolVal(x))
	case int:
		return floatValue(float64(x))
	case int8:
		return floatValue(float64(x))
	case int16:
		return floatValue(float64(x))
	case int32:
		return floatValue(float64(x))
	case int64:
		return floatValue(float64(x))
	case uint:
		return floatValue(float64(x))
	case uint8:
		return floatValue(float64(x))
	case uint16:
		return floatValue(float64(x))
	case uint32:
		return floatValue(float64(x))
	case uint64:
		return floatValue(float64(x))
	case uintptr:
		return floatValue(float64(x))
	case unsafe.Pointer:
		panic("ValueOf: unsafe.Pointer is not supported by goesm")
	case float32:
		return floatValue(float64(x))
	case float64:
		return floatValue(x)
	case string:
		return makeValue(stringVal(x))
	case []any:
		a := Global().Get("Array").New(len(x))
		for i, s := range x {
			a.SetIndex(i, s)
		}
		return a
	case map[string]any:
		o := Global().Get("Object").New()
		for k, v := range x {
			o.Set(k, v)
		}
		return o
	default:
		panic("ValueOf: invalid value")
	}
}

func floatValue(f float64) Value { return makeValue(floatVal(f)) }

// Type represents the JavaScript type of a Value.
type Type int

const (
	TypeUndefined Type = iota
	TypeNull
	TypeBoolean
	TypeNumber
	TypeString
	TypeSymbol
	TypeObject
	TypeFunction
)

func (t Type) String() string {
	switch t {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBoolean:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeObject:
		return "object"
	case TypeFunction:
		return "function"
	default:
		panic("bad type")
	}
}

func (t Type) isObject() bool {
	return t == TypeObject || t == TypeFunction
}

// Type returns the JavaScript type of the value v. It is similar to
// JavaScript's typeof operator, except that it returns TypeNull instead of
// TypeObject for null.
func (v Value) Type() Type { return Type(valueType(v.ref)) }

// Get returns the JavaScript property p of value v.
// It panics if v is not a JavaScript object.
func (v Value) Get(p string) Value {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Get", vType})
	}
	return makeValue(valueGet(v.ref, p))
}

// Set sets the JavaScript property p of value v to ValueOf(x).
// It panics if v is not a JavaScript object.
func (v Value) Set(p string, x any) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Set", vType})
	}
	valueSet(v.ref, p, ValueOf(x).ref)
}

// Delete deletes the JavaScript property p of value v.
// It panics if v is not a JavaScript object.
func (v Value) Delete(p string) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Delete", vType})
	}
	valueDelete(v.ref, p)
}

// Index returns JavaScript index i of value v.
// It panics if v is not a JavaScript object.
func (v Value) Index(i int) Value {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Index", vType})
	}
	return makeValue(valueIndex(v.ref, i))
}

// SetIndex sets the JavaScript index i of value v to ValueOf(x).
// It panics if v is not a JavaScript object.
func (v Value) SetIndex(i int, x any) {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.SetIndex", vType})
	}
	valueSetIndex(v.ref, i, ValueOf(x).ref)
}

func argRefs(args []any) []ref {
	refs := make([]ref, len(args))
	for i, a := range args {
		refs[i] = ValueOf(a).ref
	}
	return refs
}

// Length returns the JavaScript property "length" of v.
// It panics if v is not a JavaScript object.
func (v Value) Length() int {
	if vType := v.Type(); !vType.isObject() {
		panic(&ValueError{"Value.Length", vType})
	}
	return valueLength(v.ref)
}

// Call does a JavaScript call to the method m of value v with the given
// arguments. It panics if v has no method m. The arguments get mapped to
// JavaScript values according to the ValueOf function.
func (v Value) Call(m string, args ...any) Value {
	res, ok := valueCall(v.ref, m, argRefs(args))
	if !ok {
		if vType := v.Type(); !vType.isObject() {
			panic(&ValueError{"Value.Call", vType})
		}
		if propType := v.Get(m).Type(); propType != TypeFunction {
			panic("syscall/js: Value.Call: property " + m + " is not a function, got " + propType.String())
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

// Invoke does a JavaScript call of the value v with the given arguments.
// It panics if v is not a JavaScript function. The arguments get mapped to
// JavaScript values according to the ValueOf function.
func (v Value) Invoke(args ...any) Value {
	res, ok := valueInvoke(v.ref, argRefs(args))
	if !ok {
		if vType := v.Type(); vType != TypeFunction {
			panic(&ValueError{"Value.Invoke", vType})
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

// New uses JavaScript's "new" operator with value v as constructor and the
// given arguments. It panics if v is not a JavaScript function. The
// arguments get mapped to JavaScript values according to the ValueOf
// function.
func (v Value) New(args ...any) Value {
	res, ok := valueNew(v.ref, argRefs(args))
	if !ok {
		if vType := v.Type(); vType != TypeFunction {
			panic(&ValueError{"Value.New", vType})
		}
		panic(Error{makeValue(res)})
	}
	return makeValue(res)
}

func (v Value) float(method string) float64 {
	if t := v.Type(); t != TypeNumber {
		panic(&ValueError{method, t})
	}
	return valueFloat(v.ref)
}

// Float returns the value v as a float64.
// It panics if v is not a JavaScript number.
func (v Value) Float() float64 {
	return v.float("Value.Float")
}

// Int returns the value v truncated to an int.
// It panics if v is not a JavaScript number.
func (v Value) Int() int {
	return int(v.float("Value.Int"))
}

// Bool returns the value v as a bool.
// It panics if v is not a JavaScript boolean.
func (v Value) Bool() bool {
	if t := v.Type(); t != TypeBoolean {
		panic(&ValueError{"Value.Bool", t})
	}
	return valueTruthy(v.ref)
}

// Truthy returns the JavaScript "truthiness" of the value v. In JavaScript,
// false, 0, "", null, undefined, and NaN are "falsy", and everything else is
// "truthy". See https://developer.mozilla.org/en-US/docs/Glossary/Truthy.
func (v Value) Truthy() bool { return valueTruthy(v.ref) }

// String returns the value v as a string.
// String is a special case because of Go's String method convention. Unlike
// the other getters, it does not panic if v's Type is not TypeString.
// Instead, it returns a string of the form "<T>" or "<T: V>" where T is v's
// type and V is a string representation of v's value.
func (v Value) String() string {
	switch v.Type() {
	case TypeString:
		return valueString(v.ref)
	case TypeUndefined:
		return "<undefined>"
	case TypeNull:
		return "<null>"
	case TypeBoolean:
		return "<boolean: " + valueString(v.ref) + ">"
	case TypeNumber:
		return "<number: " + valueString(v.ref) + ">"
	case TypeSymbol:
		return "<symbol>"
	case TypeObject:
		return "<object>"
	case TypeFunction:
		return "<function>"
	default:
		panic("bad type")
	}
}

// InstanceOf reports whether v is an instance of type t according to
// JavaScript's instanceof operator.
func (v Value) InstanceOf(t Value) bool { return valueInstanceOf(v.ref, t.ref) }

// A ValueError occurs when a Value method is invoked on
// a Value that does not support it. Such cases are documented
// in the description of each method.
type ValueError struct {
	Method string
	Type   Type
}

func (e *ValueError) Error() string {
	return "syscall/js: call of " + e.Method + " on " + e.Type.String()
}

// CopyBytesToGo copies bytes from src to dst.
// It panics if src is not a Uint8Array or Uint8ClampedArray.
// It returns the number of bytes copied, which will be the minimum of the
// lengths of src and dst.
func CopyBytesToGo(dst []byte, src Value) int {
	n, ok := copyBytesToGo(dst, src.ref)
	if !ok {
		panic("syscall/js: CopyBytesToGo: expected src to be a Uint8Array or Uint8ClampedArray")
	}
	return n
}

// CopyBytesToJS copies bytes from src to dst.
// It panics if dst is not a Uint8Array or Uint8ClampedArray.
// It returns the number of bytes copied, which will be the minimum of the
// lengths of src and dst.
func CopyBytesToJS(dst Value, src []byte) int {
	n, ok := copyBytesToJS(dst.ref, src)
	if !ok {
		panic("syscall/js: CopyBytesToJS: expected dst to be a Uint8Array or Uint8ClampedArray")
	}
	return n
}

// Func is a wrapped Go function to be called by JavaScript.
type Func struct {
	Value // the JavaScript function that invokes the Go function
}

// FuncOf returns a function to be used by JavaScript.
//
// The Go function fn is called with the value of JavaScript's "this"
// keyword and the arguments of the invocation. The return value of the
// invocation is the result of the Go function mapped back to JavaScript
// according to ValueOf.
//
// Under goesm the JavaScript function calls fn directly. If fn blocks
// (channel operations, locks held elsewhere, time.Sleep), the JavaScript
// function returns a Promise of the result instead.
//
// Func.Release must be called to free up resources when the function will
// not be invoked any more.
func FuncOf(fn func(this Value, args []Value) any) Func {
	return Func{makeValue(makeFunc(func(this ref, args []ref) ref {
		vs := make([]Value, len(args))
		for i, a := range args {
			vs[i] = makeValue(a)
		}
		return ValueOf(fn(makeValue(this), vs)).ref
	}))}
}

// Release frees up resources allocated for the function.
// The function must not be invoked after calling Release.
// It is allowed to call Release while the function is still running.
func (c Func) Release() {
	// The JavaScript function holds fn itself, and the garbage collector
	// reclaims both.
}

// Implemented by the runtime (runtime/src/natives.ts).

func nullRef() ref
func globalRef() ref
func boolVal(b bool) ref
func floatVal(f float64) ref
func stringVal(s string) ref
func valueEqual(v, w ref) bool
func valueIsNaN(v ref) bool
func valueType(v ref) int
func valueGet(v ref, p string) ref
func valueSet(v ref, p string, x ref)
func valueDelete(v ref, p string)
func valueIndex(v ref, i int) ref
func valueSetIndex(v ref, i int, x ref)
func valueLength(v ref) int
func valueCall(v ref, m string, args []ref) (ref, bool)
func valueInvoke(v ref, args []ref) (ref, bool)
func valueNew(v ref, args []ref) (ref, bool)
func valueFloat(v ref) float64
func valueTruthy(v ref) bool
func valueString(v ref) string
func valueInstanceOf(v, t ref) bool
func copyBytesToGo(dst []byte, src ref) (int, bool)
func copyBytesToJS(dst ref, src []byte) (int, bool)
func makeFunc(fn func(this ref, args []ref) ref) ref
