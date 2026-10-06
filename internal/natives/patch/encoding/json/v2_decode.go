//go:build goesm

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/json's v2_decode.go: Unmarshal decodes into
// values of plain types (no methods, plain json tags), where nothing is
// merged into an existing map, slice, pointer or interface, in one pass in
// the runtime (jsonUnmarshal in runtime/src/json.ts). Anything else,
// including every error, goes to json v2 with v1 options as before: the
// runtime changes nothing unless it succeeds. (The calls goesm lowers go to
// the runtime's jsonDecode, which makes its errors with goesmErrors.)
package json

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	if fastUnmarshal(data, v) {
		return nil
	}
	return jsonv2.Unmarshal(data, v, DefaultOptionsV1())
}

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
