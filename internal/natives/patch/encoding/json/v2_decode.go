//go:build goesm

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/json's v2_decode.go. Unmarshal decodes into
// values of plain types (no methods, plain json tags), where nothing is
// merged into an existing map, slice, pointer or interface, in one pass in
// the runtime (jsonUnmarshal in runtime/src/json.ts): the runtime changes
// nothing unless it succeeds. (The calls goesm lowers go to the runtime's
// jsonDecode, which makes its errors with goesmErrors.)
//
// Anything else goes to the decoder of encoding/json's own implementation
// (GOEXPERIMENT=nojsonv2, decode.go), changed to return what Go's default,
// json v2 with v1 options, returns: the same errors (an UnmarshalTypeError
// with json v2's offset and JSON Pointer path, a method's error as it is,
// the first of them, with decoding going on after each) and the same
// partial results. The input is checked first by the runtime (checkValid,
// v2_scanner.go). json v2 itself, which is several times the size, decodes
// only in programs with types that have its own methods (jsonV2).
package json

import (
	"encoding"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	if fastUnmarshal(data, v) {
		return nil
	}
	if jsonV2() {
		return unmarshalV2(data, v)
	}
	return unmarshalLegacy(data, v)
}

// unmarshalV2 is Unmarshal by json v2 with v1 options, as in Go.
func unmarshalV2(data []byte, v any) error {
	return jsonv2.Unmarshal(data, v, DefaultOptionsV1())
}

// jsonV2 reports whether the program declares methods named MarshalJSONTo
// or UnmarshalJSONFrom outside encoding/json: methods of json v2's own
// interfaces, which only json v2 calls. goesm lowers it to a constant, and
// leaves out (as never called) unmarshalV2 and marshalV2 if it is false,
// or unmarshalLegacy and marshalLegacy if it is true.
func jsonV2() bool { return true }

// fastUnmarshal is Unmarshal(data, v) when the runtime decodes it and it
// succeeds; it reports false, having changed nothing, otherwise.
func fastUnmarshal(data []byte, v any) bool

// goesmErrors hands the runtime the makers of Unmarshal's errors, which it
// returns when it decodes a value itself (jsonDecode in runtime/src/json.ts).
var goesmErrors = registerErrors(
	func(msg string, offset int64) error { return &SyntaxError{msg: msg, Offset: offset} },
	func(value string, ptr any, offset int64, structName, field string) error {
		return &UnmarshalTypeError{Value: value, Type: reflect.TypeOf(ptr).Elem(), Offset: offset, Struct: structName, Field: field}
	},
	func(ptr any) error { return &InvalidUnmarshalError{reflect.TypeOf(ptr)} },
)

func registerErrors(syntax func(string, int64) error, typ func(string, any, int64, string, string) error, invalid func(any) error) bool

// ---- encoding/json's own decoder (decode.go), with json v2's errors ----

func (d *decodeState) unmarshal(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}

	d.scan.reset()
	d.scanWhile(scanSkipSpace)
	d.root = rv.Type().Elem().Name()
	// We decode rv not rv.Elem because the Unmarshaler interface
	// test must be applied at the top level of the value.
	err := d.value(rv)
	if err != nil {
		return err
	}
	return d.savedError
}

// decodeState represents the state while decoding a JSON value.
type decodeState struct {
	data                  []byte
	off                   int // next read offset in data
	opcode                int // last read result
	scan                  scanner
	path                  []string // the object names and array indices down to the value
	root                  string   // the name of the type pointed to
	quoted                []byte   // the string of a ",string" value being decoded
	savedError            error
	useNumber             bool
	disallowUnknownFields bool
}

// readIndex returns the position of the last byte read.
func (d *decodeState) readIndex() int {
	return d.off - 1
}

// phasePanicMsg is used as a panic message when we end up with something that
// shouldn't happen. It can indicate a bug in the JSON decoder, or that
// something is editing the data slice while the decoder executes.
const phasePanicMsg = "JSON decoder out of sync - data changing underfoot?"

func (d *decodeState) init(data []byte) *decodeState {
	d.data = data
	d.off = 0
	d.savedError = nil
	d.path = d.path[:0]
	return d
}

// saveError saves the first err it is called with,
// for reporting at the end of the unmarshal.
func (d *decodeState) saveError(err error) {
	if d.savedError == nil && err != nil {
		d.savedError = err
	}
}

// typeError is json v2's error for the JSON value described by value that
// cannot be stored in a t, at offset off: an UnmarshalTypeError with the
// path to the value as json v2's JSON Pointer gives it.
func (d *decodeState) typeError(value string, t reflect.Type, off int, err error) error {
	if t == numberType {
		// json v2 reports a Number's errors without offset or path.
		return &UnmarshalTypeError{Value: value, Type: t, Err: err}
	}
	e := &UnmarshalTypeError{Value: value, Type: t, Offset: int64(off), Err: err}
	if len(d.path) > 0 {
		e.Struct = d.root
		var b strings.Builder
		for i, p := range d.path {
			if i > 0 {
				b.WriteByte('.')
			}
			for j := 0; j < len(p); j++ {
				switch p[j] {
				case '~':
					b.WriteString("~0")
				case '/':
					b.WriteString("~1")
				default:
					b.WriteByte(p[j])
				}
			}
		}
		e.Field = b.String()
	}
	return e
}

// timeType returns time.Time if u is a *time.Time, which json v2 decodes
// only from strings, failing on other values with an UnmarshalTypeError.
func timeType(u Unmarshaler) reflect.Type {
	t := reflect.TypeOf(u)
	if t.Kind() == reflect.Pointer && t.Elem().Name() == "Time" && t.Elem().PkgPath() == "time" {
		return t.Elem()
	}
	return nil
}

// errNonString is json v2's error for a TextUnmarshaler given a value that
// is not a string.
var errNonString = errors.New("JSON value must be string type")

// errEmbeddedPointer is json v2's error for a value of an embedded pointer
// to an unexported struct type.
var errEmbeddedPointer = errors.New("cannot set embedded pointer to unexported struct type")

// valueKind describes the JSON value starting with c as json v2 does.
func valueKind(c byte) string {
	switch c {
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	case '[':
		return "array"
	case '{':
		return "object"
	}
	return "number"
}

// textType is the type json v2 reports in the errors of a TextUnmarshaler.
func textType(ut encoding.TextUnmarshaler) reflect.Type {
	t := reflect.TypeOf(ut)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// skip scans to the end of what was started.
func (d *decodeState) skip() {
	s, data, i := &d.scan, d.data, d.off
	depth := len(s.parseState)
	for {
		op := s.step(s, data[i])
		i++
		if len(s.parseState) < depth {
			d.off = i
			d.opcode = op
			return
		}
	}
}

// scanNext processes the byte at d.data[d.off].
func (d *decodeState) scanNext() {
	if d.off < len(d.data) {
		d.opcode = d.scan.step(&d.scan, d.data[d.off])
		d.off++
	} else {
		d.opcode = d.scan.eof()
		d.off = len(d.data) + 1 // mark processed EOF with len+1
	}
}

// scanWhile processes bytes in d.data[d.off:] until it
// receives a scan code not equal to op.
func (d *decodeState) scanWhile(op int) {
	s, data, i := &d.scan, d.data, d.off
	for i < len(data) {
		newOp := s.step(s, data[i])
		i++
		if newOp != op {
			d.opcode = newOp
			d.off = i
			return
		}
	}

	d.off = len(data) + 1 // mark processed EOF with len+1
	d.opcode = d.scan.eof()
}

// rescanLiteral is similar to scanWhile(scanContinue), but it specialises the
// common case where we're decoding a literal. The decoder scans the input
// twice, once for syntax errors and to check the length of the value, and the
// second to perform the decoding.
//
// Only in the second step do we use decodeState to tokenize literals, so we
// know there aren't any syntax errors. We can take advantage of that knowledge,
// and scan a literal's bytes much more quickly.
func (d *decodeState) rescanLiteral() {
	data, i := d.data, d.off
Switch:
	switch data[i-1] {
	case '"': // string
		for ; i < len(data); i++ {
			switch data[i] {
			case '\\':
				i++ // escaped char
			case '"':
				i++ // tokenize the closing quote too
				break Switch
			}
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-': // number
		for ; i < len(data); i++ {
			switch data[i] {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
				'.', 'e', 'E', '+', '-':
			default:
				break Switch
			}
		}
	case 't': // true
		i += len("rue")
	case 'f': // false
		i += len("alse")
	case 'n': // null
		i += len("ull")
	}
	if i < len(data) {
		d.opcode = stateEndValue(&d.scan, data[i])
	} else {
		d.opcode = scanEnd
	}
	d.off = i + 1
}

// value consumes a JSON value from d.data[d.off-1:], decoding into v, and
// reads the following byte ahead. If v is invalid, the value is discarded.
// The first byte of the value has been read already.
func (d *decodeState) value(v reflect.Value) error {
	switch d.opcode {
	default:
		panic(phasePanicMsg)

	case scanBeginArray:
		if v.IsValid() {
			if err := d.array(v); err != nil {
				return err
			}
		} else {
			d.skip()
		}
		d.scanNext()

	case scanBeginObject:
		if v.IsValid() {
			if err := d.object(v); err != nil {
				return err
			}
		} else {
			d.skip()
		}
		d.scanNext()

	case scanBeginLiteral:
		// All bytes inside literal return scanContinue op code.
		start := d.readIndex()
		d.rescanLiteral()

		if v.IsValid() {
			if err := d.literalStore(d.data[start:d.readIndex()], v, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// valueQuoted decodes into v the value of a struct field with the ",string"
// option: a JSON string holding a literal of v's type, or null.
func (d *decodeState) valueQuoted(v reflect.Value) error {
	start := d.readIndex()
	switch d.opcode {
	default:
		panic(phasePanicMsg)

	case scanBeginArray, scanBeginObject:
		indirect(v, false) // json v2 allocates pointers first
		d.saveError(d.quotedKindError(valueKind(d.data[start]), v, start+1))
		d.skip()
		d.scanNext()

	case scanBeginLiteral:
		d.rescanLiteral()
		item := d.data[start:d.readIndex()]
		switch item[0] {
		case 'n':
			return d.literalStore(nullLiteral, v, false)
		case '"':
			s, ok := unquoteBytes(item)
			if !ok {
				panic(phasePanicMsg)
			}
			d.quoted = item
			err := d.literalStore(s, v, true)
			d.quoted = nil
			return err
		}
		indirect(v, false)
		d.saveError(d.quotedKindError(valueKind(item[0]), v, d.readIndex()))
	}
	return nil
}

// quotedKindError is json v2's error for a JSON value of the kind value,
// which is not a string, for the ",string" field v.
func (d *decodeState) quotedKindError(value string, v reflect.Value, off int) error {
	t := v.Type()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == numberType {
		return &UnmarshalTypeError{Value: value, Type: t}
	}
	return d.typeError(value, t, off, nil)
}

// quotedError is json v2's error for the ",string" value d.quoted, which
// does not hold a literal of type t; err is strconv's error.
func (d *decodeState) quotedError(t reflect.Type, err error) error {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		s, _ := unquoteBytes(d.quoted)
		return d.typeError("number "+string(s), t, d.readIndex(), nil)
	}
	return d.typeError("string "+string(d.quoted), t, d.readIndex(), err)
}

// quotedStringError is json v2's error for the ",string" value d.quoted of
// a field of type t, a string type, which does not hold a JSON string.
func (d *decodeState) quotedStringError(t reflect.Type, inner []byte) error {
	msg := errUnexpectedEnd.Error()
	if len(inner) > 0 && inner[0] != '"' {
		r, _ := utf8.DecodeRune(inner)
		msg = "invalid character " + quoteChar(byte(r)) + " looking for beginning of object key string"
		if r >= utf8.RuneSelf {
			msg = "invalid character " + strconv.QuoteRune(r) + " looking for beginning of object key string"
		}
	}
	return d.typeError("string", t, d.readIndex(), errors.New(msg))
}

// indirect walks down v allocating pointers as needed,
// until it gets to a non-pointer.
// If it encounters an Unmarshaler, indirect stops and returns that.
// If decodingNull is true, indirect stops at the first settable pointer so it
// can be set to nil.
func indirect(v reflect.Value, decodingNull bool) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	// Issue #24153 indicates that it is generally not a guaranteed property
	// that you may round-trip a reflect.Value by calling Value.Addr().Elem()
	// and expect the value to still be settable for values derived from
	// unexported embedded struct fields.
	//
	// The logic below effectively does this when it first addresses the value
	// (to satisfy possible pointer methods) and continues to dereference
	// subsequent pointers as necessary.
	//
	// After the first round-trip, we set v back to the original value to
	// preserve the original RW flags contained in reflect.Value.
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Pointer && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Pointer && !e.IsNil() && (!decodingNull || e.Elem().Kind() == reflect.Pointer) {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Pointer {
			break
		}

		if decodingNull && v.CanSet() {
			break
		}

		// Prevent infinite loop if v is an interface pointing to its own address:
		//     var v any
		//     v = &v
		if v.Elem().Kind() == reflect.Interface && v.Elem().Elem().Equal(v) {
			v = v.Elem()
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := reflect.TypeAssert[Unmarshaler](v); ok {
				return u, nil, reflect.Value{}
			}
			if !decodingNull {
				if u, ok := reflect.TypeAssert[encoding.TextUnmarshaler](v); ok {
					return nil, u, reflect.Value{}
				}
			}
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return nil, nil, v
}

// array consumes an array from d.data[d.off-1:], decoding into v.
// The first byte of the array ('[') has been read already.
func (d *decodeState) array(v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		if t := timeType(u); t != nil {
			d.saveError(d.typeError("array", t, d.off, nil))
			d.skip()
			return nil
		}
		start := d.readIndex()
		d.skip()
		d.saveError(u.UnmarshalJSON(d.data[start:d.off]))
		return nil
	}
	if ut != nil {
		d.saveError(d.typeError("array", textType(ut), d.off, errNonString))
		d.skip()
		return nil
	}
	v = pv

	// Check type of target.
	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() == 0 {
			// Decoding into nil interface? Switch to non-reflect code.
			ai := d.arrayInterface()
			v.Set(reflect.ValueOf(ai))
			return nil
		}
		d.saveError(d.typeError("array", v.Type(), d.readIndex(), nil))
		d.skip()
		return nil
	default:
		d.saveError(d.typeError("array", v.Type(), d.off, nil))
		d.skip()
		return nil
	case reflect.Array, reflect.Slice:
		break
	}

	i := 0
	for {
		// Look ahead for ] - can only happen on first iteration.
		d.scanWhile(scanSkipSpace)
		if d.opcode == scanEndArray {
			break
		}

		// Expand slice length, growing the slice if necessary.
		if v.Kind() == reflect.Slice {
			if i >= v.Cap() {
				v.Grow(1)
			}
			if i >= v.Len() {
				v.SetLen(i + 1)
			}
		}

		if i < v.Len() {
			// Decode into element.
			d.path = append(d.path, strconv.Itoa(i))
			if err := d.value(v.Index(i)); err != nil {
				return err
			}
			d.path = d.path[:len(d.path)-1]
		} else {
			// Ran out of fixed array: skip.
			if err := d.value(reflect.Value{}); err != nil {
				return err
			}
		}
		i++

		// Next token must be , or ].
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode == scanEndArray {
			break
		}
		if d.opcode != scanArrayValue {
			panic(phasePanicMsg)
		}
	}

	if i < v.Len() {
		if v.Kind() == reflect.Array {
			for ; i < v.Len(); i++ {
				v.Index(i).SetZero() // zero remainder of array
			}
		} else {
			v.SetLen(i) // truncate the slice
		}
	}
	if i == 0 && v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
	return nil
}

var nullLiteral = []byte("null")
var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()

// object consumes an object from d.data[d.off-1:], decoding into v.
// The first byte ('{') of the object has been read already.
func (d *decodeState) object(v reflect.Value) error {
	// Check for unmarshaler.
	u, ut, pv := indirect(v, false)
	if u != nil {
		if t := timeType(u); t != nil {
			d.saveError(d.typeError("object", t, d.off, nil))
			d.skip()
			return nil
		}
		start := d.readIndex()
		d.skip()
		d.saveError(u.UnmarshalJSON(d.data[start:d.off]))
		return nil
	}
	if ut != nil {
		d.saveError(d.typeError("object", textType(ut), d.off, errNonString))
		d.skip()
		return nil
	}
	v = pv
	t := v.Type()

	// Decoding into nil interface? Switch to non-reflect code.
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		oi := d.objectInterface()
		v.Set(reflect.ValueOf(oi))
		return nil
	}

	var fields structFields

	// Check type of target: a struct or a map. As json v2 does, a map whose
	// keys are not strings, integers or TextUnmarshalers fails on each key.
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:
		fields = cachedTypeFields(t)
		if fields.errFormat != nil {
			d.saveError(d.typeError("object", fields.errFormatType, d.off, fields.errFormat))
			d.skip()
			return nil
		}
		// ok
	case reflect.Interface:
		d.saveError(d.typeError("object", t, d.readIndex(), nil))
		d.skip()
		return nil
	default:
		d.saveError(d.typeError("object", t, d.off, nil))
		d.skip()
		return nil
	}

	var mapElem reflect.Value

	for {
		// Read opening " of string key or closing }.
		d.scanWhile(scanSkipSpace)
		if d.opcode == scanEndObject {
			// closing } - can only happen on first iteration.
			break
		}
		if d.opcode != scanBeginLiteral {
			panic(phasePanicMsg)
		}

		// Read key.
		start := d.readIndex()
		d.rescanLiteral()
		item := d.data[start:d.readIndex()]
		key, ok := unquoteBytes(item)
		if !ok {
			panic(phasePanicMsg)
		}
		d.path = append(d.path, string(key))

		// Figure out field corresponding to key.
		var subv reflect.Value
		var kv reflect.Value
		var keyErr, fieldErr error
		destring := false // whether the value is wrapped in a string to be decoded first

		if v.Kind() == reflect.Map {
			// The key is decoded first, as json v2 does: the value of a key
			// that fails is skipped.
			kv, keyErr = d.mapKey(t.Key(), item, key, d.readIndex())
			if keyErr != nil {
				d.saveError(keyErr)
			} else {
				elemType := t.Elem()
				if !mapElem.IsValid() {
					mapElem = reflect.New(elemType).Elem()
				} else {
					mapElem.SetZero()
				}
				subv = mapElem
			}
		} else {
			f := fields.byExactName[string(key)]
			if f == nil {
				// As json v2 does, the first field in breadth-first
				// order that matches without case, unless its tag says
				// case:strict.
				for _, ff := range fields.byFoldedName[string(foldName(key))] {
					if !ff.caseStrict {
						f = ff
						break
					}
				}
			}
			if f != nil {
				subv = v
				destring = f.quoted
				for _, ind := range f.index {
					if subv.Kind() == reflect.Pointer {
						if subv.IsNil() {
							// If a struct embeds a pointer to an unexported type,
							// it is not possible to set a newly allocated value
							// since the field is unexported.
							//
							// See https://golang.org/issue/21357
							if !subv.CanSet() {
								fieldErr = errEmbeddedPointer
								// Invalidate subv to ensure d.value(subv) skips over
								// the JSON value without assigning it to subv.
								subv = reflect.Value{}
								destring = false
								break
							}
							subv.Set(reflect.New(subv.Type().Elem()))
						}
						subv = subv.Elem()
					}
					subv = subv.Field(ind)
				}
			} else if d.disallowUnknownFields {
				d.saveError(fmt.Errorf("json: unknown field %q", key))
			}
		}

		// Read : before value.
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode != scanObjectKey {
			panic(phasePanicMsg)
		}
		d.scanWhile(scanSkipSpace)
		if fieldErr != nil {
			d.saveError(d.typeError(valueKind(d.data[d.readIndex()]), t, d.readIndex(), fieldErr))
		}

		if destring {
			if err := d.valueQuoted(subv); err != nil {
				return err
			}
		} else {
			if err := d.value(subv); err != nil {
				return err
			}
		}

		// Write value back to map;
		// if using struct, subv points into struct already.
		if v.Kind() == reflect.Map && keyErr == nil {
			v.SetMapIndex(kv, subv)
		}
		d.path = d.path[:len(d.path)-1]

		// Next token must be , or }.
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode == scanEndObject {
			break
		}
		if d.opcode != scanObjectValue {
			panic(phasePanicMsg)
		}
	}
	return nil
}

// mapKey decodes the object name item (key unquoted), which ends at end,
// into a map key of type kt.
func (d *decodeState) mapKey(kt reflect.Type, item, key []byte, end int) (reflect.Value, error) {
	if reflect.PointerTo(kt).Implements(textUnmarshalerType) {
		kv := reflect.New(kt)
		if err := kv.Interface().(encoding.TextUnmarshaler).UnmarshalText(key); err != nil {
			return reflect.Value{}, err
		}
		return kv.Elem(), nil
	}
	switch kt.Kind() {
	case reflect.String:
		kv := reflect.New(kt).Elem()
		kv.SetString(string(key))
		return kv, nil
	case reflect.Interface:
		if kt.NumMethod() == 0 {
			return reflect.ValueOf(string(key)), nil
		}
	case reflect.Float32, reflect.Float64:
		s := string(key)
		n, err := strconv.ParseFloat(s, kt.Bits())
		if err != nil {
			return reflect.Value{}, d.typeError("number "+s, kt, end, nil)
		}
		kv := reflect.New(kt).Elem()
		kv.SetFloat(n)
		return kv, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s := string(key)
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || reflect.Zero(kt).OverflowInt(n) {
			return reflect.Value{}, d.typeError("number "+s, kt, end, nil)
		}
		kv := reflect.New(kt).Elem()
		kv.SetInt(n)
		return kv, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s := string(key)
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || reflect.Zero(kt).OverflowUint(n) {
			return reflect.Value{}, d.typeError("number "+s, kt, end, nil)
		}
		kv := reflect.New(kt).Elem()
		kv.SetUint(n)
		return kv, nil
	}
	return reflect.Value{}, d.typeError("string", kt, end, nil)
}

// convertNumber converts the number literal s to a float64 or a Number
// depending on the setting of d.useNumber.
func (d *decodeState) convertNumber(s string) (any, error) {
	if d.useNumber {
		return Number(s), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// As json v2 does, an out of range number is stored as ±Inf.
		return f, d.typeError("number "+s, reflect.TypeFor[float64](), d.readIndex(), nil)
	}
	return f, nil
}

// literalStore decodes a literal stored in item into v.
//
// fromQuoted indicates whether this literal came from unwrapping a
// string from the ",string" struct tag option (d.quoted).
func (d *decodeState) literalStore(item []byte, v reflect.Value, fromQuoted bool) error {
	isNull := len(item) > 0 && item[0] == 'n' && (!fromQuoted || string(item) == "null")
	u, ut, pv := indirect(v, isNull)
	if u != nil {
		if t := timeType(u); t != nil && !fromQuoted && item[0] != 'n' {
			// json v2 decodes a time.Time with its UnmarshalText,
			// which leaves it unchanged on errors.
			if item[0] != '"' {
				d.saveError(d.typeError(valueKind(item[0]), t, d.readIndex(), nil))
				return nil
			}
			s, ok := unquoteBytes(item)
			if !ok {
				panic(phasePanicMsg)
			}
			tv := reflect.New(t)
			if err := tv.Interface().(encoding.TextUnmarshaler).UnmarshalText(s); err != nil {
				d.saveError(err)
				return nil
			}
			reflect.ValueOf(u).Elem().Set(tv.Elem())
			return nil
		}
		d.saveError(u.UnmarshalJSON(item))
		return nil
	}
	if ut != nil {
		if len(item) == 0 || item[0] != '"' {
			if fromQuoted {
				d.saveError(d.quotedError(textType(ut), strconv.ErrSyntax))
				return nil
			}
			d.saveError(d.typeError(valueKind(item[0]), textType(ut), d.readIndex(), errNonString))
			return nil
		}
		s, ok := unquoteBytes(item)
		if !ok {
			d.saveError(d.quotedError(textType(ut), strconv.ErrSyntax))
			return nil
		}
		d.saveError(ut.UnmarshalText(s))
		return nil
	}

	v = pv
	if fromQuoted {
		return d.quotedStore(item, v)
	}
	// The offset of an error: the end of the value, or its start for an
	// interface type with methods.
	end := d.readIndex()
	if v.Kind() == reflect.Interface {
		end -= len(item)
	}

	switch c := item[0]; c {
	case 'n': // null
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
			v.SetZero()
			// otherwise, ignore null for primitives/string
		}
	case 't', 'f': // true, false
		value := item[0] == 't'
		switch v.Kind() {
		default:
			d.saveError(d.typeError("bool", v.Type(), end, nil))
		case reflect.Bool:
			v.SetBool(value)
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(value))
			} else {
				d.saveError(d.typeError("bool", v.Type(), end, nil))
			}
		}

	case '"': // string
		s, ok := unquoteBytes(item)
		if !ok {
			panic(phasePanicMsg)
		}
		switch v.Kind() {
		default:
			d.saveError(d.typeError("string", v.Type(), end, nil))
		case reflect.Slice:
			if v.Type().Elem().Kind() != reflect.Uint8 {
				d.saveError(d.typeError("string", v.Type(), end, nil))
				break
			}
			b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
			n, err := base64.StdEncoding.Decode(b, s)
			if err != nil {
				d.saveError(d.typeError("string", v.Type(), end, err))
				break
			}
			v.SetBytes(b[:n])
		case reflect.String:
			t := string(s)
			if v.Type() == numberType && !isValidNumber(t) {
				d.saveError(&UnmarshalTypeError{Value: "string " + string(item), Type: numberType, Err: strconv.ErrSyntax})
				break
			}
			v.SetString(t)
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(string(s)))
			} else {
				d.saveError(d.typeError("string", v.Type(), end, nil))
			}
		}

	default: // number
		if c != '-' && (c < '0' || c > '9') {
			panic(phasePanicMsg)
		}
		switch v.Kind() {
		default:
			if v.Kind() == reflect.String && v.Type() == numberType {
				// s must be a valid number, because it's
				// already been tokenized.
				v.SetString(string(item))
				break
			}
			d.saveError(d.typeError("number", v.Type(), end, nil))
		case reflect.Interface:
			if v.NumMethod() != 0 {
				d.saveError(d.typeError("number", v.Type(), end, nil))
				break
			}
			n, err := d.convertNumber(string(item))
			d.saveError(err)
			v.Set(reflect.ValueOf(n))

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(string(item), 10, 64)
			if err != nil || v.OverflowInt(n) {
				d.saveError(d.typeError("number "+string(item), v.Type(), end, nil))
				break
			}
			v.SetInt(n)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n, err := strconv.ParseUint(string(item), 10, 64)
			if err != nil || v.OverflowUint(n) {
				d.saveError(d.typeError("number "+string(item), v.Type(), end, nil))
				break
			}
			v.SetUint(n)

		case reflect.Float32, reflect.Float64:
			// As json v2 does, an out of range number is stored as ±Inf.
			n, err := strconv.ParseFloat(string(item), v.Type().Bits())
			if err != nil {
				d.saveError(d.typeError("number "+string(item), v.Type(), end, nil))
			}
			v.SetFloat(n)
		}
	}
	return nil
}

// quotedStore decodes into v the literal item that the ",string" value
// d.quoted holds.
func (d *decodeState) quotedStore(item []byte, v reflect.Value) error {
	t := v.Type()
	switch v.Kind() {
	case reflect.String:
		if t == numberType {
			if !isValidNumber(string(item)) {
				d.saveError(&UnmarshalTypeError{Value: "string " + string(d.quoted), Type: numberType, Err: strconv.ErrSyntax})
				break
			}
			v.SetString(string(item))
			break
		}
		if string(item) == "null" {
			break
		}
		s, ok := unquoteBytes(item)
		if !ok {
			d.saveError(d.quotedStringError(t, item))
			break
		}
		v.SetString(string(s))
	case reflect.Bool:
		switch string(item) {
		case "true", "false":
			v.SetBool(item[0] == 't')
		case "null":
		default:
			d.saveError(d.quotedError(t, strconv.ErrSyntax))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if string(item) == "null" {
			break
		}
		n, err := strconv.ParseInt(string(item), 10, 64)
		if err != nil || v.OverflowInt(n) {
			d.saveError(d.quotedError(t, err))
			break
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if string(item) == "null" {
			break
		}
		n, err := strconv.ParseUint(string(item), 10, 64)
		if err != nil || v.OverflowUint(n) {
			d.saveError(d.quotedError(t, err))
			break
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		if string(item) == "null" {
			break
		}
		n, err := strconv.ParseFloat(string(item), t.Bits())
		if err != nil || v.OverflowFloat(n) {
			d.saveError(d.quotedError(t, err))
			break
		}
		v.SetFloat(n)
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		if string(item) == "null" {
			v.SetZero()
		}
	}
	return nil
}

// The xxxInterface routines build up a value to be stored
// in an empty interface. They are not strictly necessary,
// but they avoid the weight of reflection in this common case.

// valueInterface is like value but returns any.
func (d *decodeState) valueInterface() (val any) {
	switch d.opcode {
	default:
		panic(phasePanicMsg)
	case scanBeginArray:
		val = d.arrayInterface()
		d.scanNext()
	case scanBeginObject:
		val = d.objectInterface()
		d.scanNext()
	case scanBeginLiteral:
		val = d.literalInterface()
	}
	return
}

// arrayInterface is like array but returns []any.
func (d *decodeState) arrayInterface() []any {
	var v = make([]any, 0)
	for {
		// Look ahead for ] - can only happen on first iteration.
		d.scanWhile(scanSkipSpace)
		if d.opcode == scanEndArray {
			break
		}

		d.path = append(d.path, strconv.Itoa(len(v)))
		v = append(v, d.valueInterface())
		d.path = d.path[:len(d.path)-1]

		// Next token must be , or ].
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode == scanEndArray {
			break
		}
		if d.opcode != scanArrayValue {
			panic(phasePanicMsg)
		}
	}
	return v
}

// objectInterface is like object but returns map[string]any.
func (d *decodeState) objectInterface() map[string]any {
	m := make(map[string]any)
	for {
		// Read opening " of string key or closing }.
		d.scanWhile(scanSkipSpace)
		if d.opcode == scanEndObject {
			// closing } - can only happen on first iteration.
			break
		}
		if d.opcode != scanBeginLiteral {
			panic(phasePanicMsg)
		}

		// Read string key.
		start := d.readIndex()
		d.rescanLiteral()
		item := d.data[start:d.readIndex()]
		key, ok := unquote(item)
		if !ok {
			panic(phasePanicMsg)
		}

		// Read : before value.
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode != scanObjectKey {
			panic(phasePanicMsg)
		}
		d.scanWhile(scanSkipSpace)

		// Read value.
		d.path = append(d.path, key)
		m[key] = d.valueInterface()
		d.path = d.path[:len(d.path)-1]

		// Next token must be , or }.
		if d.opcode == scanSkipSpace {
			d.scanWhile(scanSkipSpace)
		}
		if d.opcode == scanEndObject {
			break
		}
		if d.opcode != scanObjectValue {
			panic(phasePanicMsg)
		}
	}
	return m
}

// literalInterface consumes and returns a literal from d.data[d.off-1:] and
// it reads the following byte ahead. The first byte of the literal has been
// read already (that's how the caller knows it's a literal).
func (d *decodeState) literalInterface() any {
	// All bytes inside literal return scanContinue op code.
	start := d.readIndex()
	d.rescanLiteral()

	item := d.data[start:d.readIndex()]

	switch c := item[0]; c {
	case 'n': // null
		return nil

	case 't', 'f': // true, false
		return c == 't'

	case '"': // string
		s, ok := unquote(item)
		if !ok {
			panic(phasePanicMsg)
		}
		return s

	default: // number
		if c != '-' && (c < '0' || c > '9') {
			panic(phasePanicMsg)
		}
		n, err := d.convertNumber(string(item))
		if err != nil {
			d.saveError(err)
		}
		return n
	}
}

// getu4 decodes \uXXXX from the beginning of s, returning the hex value,
// or it returns -1.
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c = c - '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}

// unquote converts a quoted JSON string literal s into an actual string t.
// The rules are different than for Go, so cannot use strconv.Unquote.
func unquote(s []byte) (t string, ok bool) {
	s, ok = unquoteBytes(s)
	t = string(s)
	return
}

func unquoteBytes(s []byte) (t []byte, ok bool) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return
	}
	s = s[1 : len(s)-1]

	// Check for unusual characters. If there are none,
	// then no unquoting is needed, so return a slice of the
	// original bytes.
	r := 0
	for r < len(s) {
		c := s[r]
		if c == '\\' || c == '"' || c < ' ' {
			break
		}
		rr, size := utf8.DecodeRune(s[r:])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		r += size
	}
	if r == len(s) {
		return s, true
	}

	b := make([]byte, len(s)+2*utf8.UTFMax)
	w := copy(b, s[0:r])
	for r < len(s) {
		// Out of room? Can only happen if s is full of
		// malformed UTF-8 and we're replacing each
		// byte with RuneError.
		if w >= len(b)-2*utf8.UTFMax {
			nb := make([]byte, (len(b)+utf8.UTFMax)*2)
			copy(nb, b[0:w])
			b = nb
		}
		switch c := s[r]; {
		case c == '\\':
			r++
			if r >= len(s) {
				return
			}
			switch s[r] {
			default:
				return
			case '"', '\\', '/', '\'':
				b[w] = s[r]
				r++
				w++
			case 'b':
				b[w] = '\b'
				r++
				w++
			case 'f':
				b[w] = '\f'
				r++
				w++
			case 'n':
				b[w] = '\n'
				r++
				w++
			case 'r':
				b[w] = '\r'
				r++
				w++
			case 't':
				b[w] = '\t'
				r++
				w++
			case 'u':
				r--
				rr := getu4(s[r:])
				if rr < 0 {
					return
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != unicode.ReplacementChar {
						// A valid pair; consume.
						r += 6
						w += utf8.EncodeRune(b[w:], dec)
						break
					}
					// Invalid surrogate; fall back to replacement rune.
					rr = unicode.ReplacementChar
				}
				w += utf8.EncodeRune(b[w:], rr)
			}

		// Quote, control characters are invalid.
		case c == '"', c < ' ':
			return

		// ASCII
		case c < utf8.RuneSelf:
			b[w] = c
			r++
			w++

		// Coerce to well-formed UTF-8.
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			w += utf8.EncodeRune(b[w:], rr)
		}
	}
	return b[0:w], true
}

// unmarshalLegacy is Unmarshal with the decoder of encoding/json's own
// implementation (GOEXPERIMENT=nojsonv2), with json v2's errors.
func unmarshalLegacy(data []byte, v any) error {
	// json v2 checks v before the input.
	if rv := reflect.ValueOf(v); rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}
	if err := checkValid(data); err != nil {
		return err
	}
	var d decodeState
	d.init(data)
	return d.unmarshal(v)
}
