// Command intops exercises integer shifts, division and bitwise operations
// by constants and variables across widths, including the edge values the
// inlined forms must keep exact. int and uint values stay below 2^53, where
// goesm's numbers are exact.
package main

import (
	"fmt"
	"math"
)

//go:noinline
func id[T any](x T) T { return x }

func main() {
	// Shifts by constants and by variables.
	var u8 uint8 = 0xb5
	var i8 int8 = -77
	var u16 uint16 = 0xbeef
	var i32 int32 = math.MinInt32 + 5
	var u32 uint32 = 0xdeadbeef
	var i int = -1 << 40
	var u uint = 1<<51 + 12345
	var i64 int64 = math.MinInt64 + 3
	var u64 uint64 = math.MaxUint64 - 7
	fmt.Println(u8<<3, u8>>3, i8<<2, i8>>2, u16<<9, u16>>9)
	fmt.Println(i32<<1, i32>>1, i32>>31, u32<<4, u32>>4, u32>>31)
	fmt.Println(i<<3, i>>3, i>>62, i>>63, u>>1, u>>63, u<<1)
	fmt.Println(i64<<1, i64>>1, i64>>63, u64<<3, u64>>3, u64>>63)
	for _, n := range []uint{0, 1, 7, 8, 15, 31, 32, 33, 52, 53, 63, 64, 65} {
		fmt.Println(n, u8<<n, u8>>n, i8>>n, u32<<n, u32>>n, i32>>n, i>>n, u>>n, i64>>n, u64<<n, u64>>n)
	}

	// Division and remainder by constants: truncation toward zero, the
	// sign of the dividend, and no negative zero.
	for _, x := range []int{-7, -6, -1, 0, 1, 6, 7, 1<<53 - 1, -(1 << 53) + 1} {
		q, r := x/2, x%2
		fmt.Println(x, q, r, x/-3, x%-3, x/1000, x%1000, math.Signbit(float64(q)), math.Signbit(float64(r)), math.Signbit(float64(x/id(-3))))
	}
	for _, x := range []int32{math.MinInt32, -5, 5, math.MaxInt32} {
		fmt.Println(x/2, x%2, x/-1, x%-1, x/7, x%7)
	}
	for _, x := range []uint32{0, 5, math.MaxUint32} {
		fmt.Println(x/2, x%2, x/7, x%1000)
	}
	for _, x := range []int8{-128, -5, 127} {
		fmt.Println(x/-1, x%3, x/3)
	}
	fmt.Println(math.Signbit(float64(id(-1)/2)), math.Signbit(float64(id(-4)%2)))

	// Bitwise operations on int and uint across the 32-bit boundary.
	vals := []int{0, 1, -1, 5, -5, 1 << 31, -(1 << 31), 1<<32 - 1, 1 << 32, -(1 << 32), 1<<52 + 3, -(1<<52 + 3), 1<<53 - 1, -(1<<53 - 1)}
	for _, a := range vals {
		for _, b := range vals[:6] {
			fmt.Println(a&b, a|b, a^b, a&^b, ^a)
		}
	}
	uvals := []uint{0, 1, 1<<32 - 1, 1 << 32, 1<<52 + 2, 1<<53 - 1}
	for _, a := range uvals {
		for _, b := range uvals {
			fmt.Println(a&b, a|b, a^b, a&^b)
		}
	}

	// Indexing strings.
	s := "héllo"
	n := 0
	for k := 0; k < len(s); k++ {
		n += int(s[k])
	}
	fmt.Println(n, s[0], s[len(s)-1], "abc"[1])
	defer func() { fmt.Println("recovered:", recover()) }()
	k := id(5)
	fmt.Println(s[k+1])
}
