//go:build goesm

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of encoding/json's v2_stream.go. Encoder.Encode encodes
// values of plain types in the runtime, as Marshal does (v2_encode.go), and
// everything else with json v2, as in Go.
package json

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

// Encode writes the JSON encoding of v to the stream,
// followed by a newline character.
//
// See the documentation for [Marshal] for details about the
// conversion of Go values to JSON.
func (enc *Encoder) Encode(v any) error {
	if enc.err != nil {
		return enc.err
	}

	escape, _ := jsonv2.GetOption(enc.opts, jsontext.EscapeForHTML)
	if b := fastEncode(v, escape); b != nil {
		return enc.write(b)
	}
	e := export.GetBufferedEncoder(enc.opts)
	defer export.PutBufferedEncoder(e)
	if err := jsonv2.MarshalEncode(e, v); err != nil {
		return err
	}
	b := export.Encoder(e).Buf // b must not leak current scope
	return enc.write(b)
}

// write indents b if the encoder indents and writes it with a newline.
func (enc *Encoder) write(b []byte) error {
	if len(enc.indentPrefix)+len(enc.indentValue) > 0 {
		enc.indentBuf.Reset()
		if err := Indent(&enc.indentBuf, b, enc.indentPrefix, enc.indentValue); err != nil {
			return err
		}
		b = enc.indentBuf.Bytes()
	}
	b = append(b, '\n')

	if _, err := enc.w.Write(b); err != nil {
		enc.err = err
		return err
	}
	return nil
}

// fastEncode is Marshal(v) for the values the runtime encodes, without
// escaping <, > and & unless escapeHTML is set, or nil.
func fastEncode(v any, escapeHTML bool) []byte
