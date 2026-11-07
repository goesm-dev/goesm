// Command byteorder checks encoding/binary's 64-bit loads and stores
// (patched to work on 32-bit halves) and comparisons of byte slices
// converted to strings (lowered without making the strings).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc64"
)

func main() {
	vals := []uint64{0, 1, 0xff, 0x80000000, 0xffffffff, 0x100000000, 0x0123456789abcdef, 0x8000000000000000, 0xfedcba9876543210, ^uint64(0)}
	b := make([]byte, 8)
	var h uint64
	for _, v := range vals {
		binary.LittleEndian.PutUint64(b, v)
		le := binary.LittleEndian.Uint64(b)
		fmt.Printf("%x %x", b, le)
		binary.BigEndian.PutUint64(b, v)
		be := binary.BigEndian.Uint64(b)
		fmt.Printf(" %x %x", b, be)
		a := binary.LittleEndian.AppendUint64([]byte{9}, v)
		a = binary.BigEndian.AppendUint64(a, v)
		fmt.Printf(" %x\n", a)
		h = h*31 + le ^ be
	}
	fmt.Println(h, crc64.Checksum([]byte("byteorder"), crc64.MakeTable(crc64.ECMA)))

	x, y := []byte("token"), []byte("tokens")
	var nilb []byte
	fmt.Println(bytes.Equal(x, y[:5]), bytes.Equal(x, y), bytes.Equal(nilb, []byte{}), bytes.Equal(nilb, x))
	fmt.Println(string(x) == "token", string(x) != "token", string(y) == "token", "tokens" == string(y), string(nilb) == "")
	z := []byte{0xe3, 0x81, 0x82}
	fmt.Println(string(z) == "あ", string(z[:2]) == "あ", string(z) == string([]byte("あ")))
	same := x[1:4]
	fmt.Println(string(same) == string(x[1:4]), string(same) == string(y[1:3]))
}
