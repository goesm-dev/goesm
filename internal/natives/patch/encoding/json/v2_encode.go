//go:build goesm

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/json's v2_encode.go: Marshal encodes values of
// plain types (no methods, plain json tags) in one pass in the runtime
// (jsonMarshal in runtime/src/json.ts), and leaves everything else to
// json v2 with v1 options as before.
package json

// Marshal returns the JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	if b := fastMarshal(v); b != nil {
		return b, nil
	}
	return jsonv2.Marshal(v, DefaultOptionsV1())
}

// fastMarshal is Marshal(v) for the values the runtime encodes, or nil.
func fastMarshal(v any) []byte
