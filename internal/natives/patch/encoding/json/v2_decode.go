//go:build goesm

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/json's v2_decode.go: Unmarshal decodes into
// values of plain types (no methods, plain json tags), where nothing is
// merged into an existing map, slice, pointer or interface, in one pass in
// the runtime (jsonUnmarshal in runtime/src/json.ts). Anything else,
// including every error, goes to json v2 with v1 options as before: the
// runtime changes nothing unless it succeeds.
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
