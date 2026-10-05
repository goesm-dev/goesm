//go:build js && wasm

// Package rpc calls TaskService.ListTasks as ../../rpc does, in the Connect
// protocol with the binary protobuf encoding, but with the two messages
// encoded and decoded by hand and the request made with the page's fetch
// through syscall/js, instead of with connect-go, the protobuf runtime
// and net/http, whose reflection-based code a bundle would carry.
// js/impl/rpc.mjs is the same with connect-es.
package rpc

import (
	"errors"
	"strconv"
	"syscall/js"
)

// Summary is what List returns about the tasks the server sent.
type Summary struct {
	Count int    `json:"count"`
	Done  int    `json:"done"`
	First string `json:"first"`
	Tags  int    `json:"tags"`
}

// List calls TaskService.ListTasks at baseURL.
func List(baseURL, owner string, limit int) (Summary, error) {
	// ListTasksRequest: owner = 1, limit = 2.
	req := appendString(nil, 1, owner)
	if limit != 0 {
		req = appendInt32(appendVarint(req, 2<<3|0), int32(limit))
	}
	res, err := post(baseURL+"/task.v1.TaskService/ListTasks", req)
	if err != nil {
		return Summary{}, err
	}
	var s Summary
	// ListTasksResponse: repeated Task tasks = 1.
	err = fields(res, func(num int, typ byte, b []byte, _ bool) error {
		if num != 1 || typ != 2 {
			return nil
		}
		first := s.Count == 0
		s.Count++
		// Task: title = 2, done = 3, repeated tags = 5.
		return fields(b, func(num int, typ byte, b []byte, set bool) error {
			switch {
			case num == 2 && typ == 2 && first:
				s.First = string(b)
			case num == 3 && typ == 0 && set:
				s.Done++
			case num == 5 && typ == 2:
				s.Tags++
			}
			return nil
		})
	})
	return s, err
}

var errProto = errors.New("rpc: malformed protobuf message")

// Like protobuf-es, the code reads and writes tags and lengths as 32-bit
// varints, and needs no 64-bit arithmetic for the fields it reads.

func appendVarint(b []byte, v uint32) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// appendInt32 appends v as protobuf encodes an int32: a negative one as the
// ten bytes of its sign extension to 64 bits.
func appendInt32(b []byte, v int32) []byte {
	if v >= 0 {
		return appendVarint(b, uint32(v))
	}
	u := uint64(int64(v))
	for u >= 0x80 {
		b = append(b, byte(u)|0x80)
		u >>= 7
	}
	return append(b, byte(u))
}

func appendString(b []byte, num int, s string) []byte {
	b = appendVarint(b, uint32(num)<<3|2)
	b = appendVarint(b, uint32(len(s)))
	return append(b, s...)
}

// varint32 reads a varint that must fit in 32 bits, such as a tag or a
// length.
func varint32(b []byte) (uint32, int) {
	var v uint32
	for i := 0; i < len(b) && i < 5; i++ {
		c := b[i]
		if i == 4 && c > 0x0f {
			return 0, -1
		}
		v |= uint32(c&0x7f) << (7 * i)
		if c < 0x80 {
			return v, i + 1
		}
	}
	return 0, -1
}

// skipVarint reads a varint of up to 64 bits and reports whether it is
// nonzero, which is all the code needs of a bool's or an enum's value.
func skipVarint(b []byte) (nonzero bool, n int) {
	for i := 0; i < len(b) && i < 10; i++ {
		nonzero = nonzero || b[i]&0x7f != 0
		if b[i] < 0x80 {
			return nonzero, i + 1
		}
	}
	return false, -1
}

// fields calls f with each field of the message b: its number, its wire
// type, and its bytes (length-delimited fields) or whether its value is
// nonzero (varints).
func fields(b []byte, f func(num int, typ byte, b []byte, set bool) error) error {
	for len(b) > 0 {
		tag, n := varint32(b)
		if n < 0 {
			return errProto
		}
		b = b[n:]
		num, typ := int(tag>>3), byte(tag&7)
		var val []byte
		var set bool
		switch typ {
		case 0:
			if set, n = skipVarint(b); n < 0 {
				return errProto
			}
		case 1, 5:
			n = 8
			if typ == 5 {
				n = 4
			}
		case 2:
			l, m := varint32(b)
			if m < 0 || int64(l) > int64(len(b)-m) {
				return errProto
			}
			val, n = b[m:m+int(l)], m+int(l)
		default:
			return errProto
		}
		if n > len(b) {
			return errProto
		}
		b = b[n:]
		if err := f(num, typ, val, set); err != nil {
			return err
		}
	}
	return nil
}

// post sends body in a unary Connect request and returns the response's
// body.
func post(url string, body []byte) ([]byte, error) {
	data := js.Global().Get("Uint8Array").New(len(body))
	js.CopyBytesToJS(data, body)
	headers := js.Global().Get("Object").New()
	headers.Set("content-type", "application/proto")
	headers.Set("connect-protocol-version", "1")
	init := js.Global().Get("Object").New()
	init.Set("method", "POST")
	init.Set("headers", headers)
	init.Set("body", data)
	res, err := await(js.Global().Call("fetch", url, init))
	if err != nil {
		return nil, err
	}
	if status := res.Get("status").Int(); status != 200 {
		return nil, errors.New("rpc: HTTP status " + strconv.Itoa(status))
	}
	buf, err := await(res.Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	b := js.Global().Get("Uint8Array").New(buf)
	out := make([]byte, b.Length())
	js.CopyBytesToGo(out, b)
	return out, nil
}

// await waits for the promise p to settle.
func await(p js.Value) (js.Value, error) {
	done := make(chan struct{})
	var v js.Value
	var err error
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v = args[0]
		close(done)
		return nil
	})
	defer then.Release()
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		err = js.Error{Value: args[0]}
		close(done)
		return nil
	})
	defer catch.Release()
	p.Call("then", then, catch)
	<-done
	return v, err
}
