// Command sortbuiltin covers slices.Sort of integers and strings, which the
// runtime sorts with the engine's sort (native$slices$sortBuiltin): whole
// backing arrays and sub-slices, integers that fit 32 bits and ones that do
// not, and the kinds with their own typed arrays.
package main

import (
	"fmt"
	"math"
	"slices"
	"sort"
)

func main() {
	s := []string{"b", "é", "a", "B", "", "ab", "a\x00"}
	sort.Strings(s)
	fmt.Printf("%q\n", s)

	whole := []string{"z", "y", "x", "w"}
	sub := whole[1:3]
	slices.Sort(sub)
	fmt.Println(whole, sub)

	spare := make([]string, 3, 8)
	spare[0], spare[1], spare[2] = "c", "a", "b"
	slices.Sort(spare)
	fmt.Println(spare, len(spare), cap(spare))

	ints := []int{5, -3, 1 << 31, -1 << 31, 0, math.MaxInt32, -7, 1 << 40, 2}
	sort.Ints(ints)
	fmt.Println(ints)

	small := []int{9, -2, 4, 0, -2, 7}
	head := small[:4]
	slices.Sort(head)
	fmt.Println(small)

	u32 := []uint32{math.MaxUint32, 1, 1 << 31, 0}
	slices.Sort(u32)
	fmt.Println(u32)

	i8 := []int8{-128, 127, 0, -1}
	slices.Sort(i8)
	fmt.Println(i8)

	b := []byte("sorted")
	slices.Sort(b)
	fmt.Println(string(b))

	i64 := []int64{math.MaxInt64, math.MinInt64, 0, -1}
	slices.Sort(i64)
	fmt.Println(i64)

	u64 := []uint64{math.MaxUint64, 0, 1 << 63}
	slices.Sort(u64)
	fmt.Println(u64)

	var none []int
	slices.Sort(none)
	one := []string{"x"}
	slices.Sort(one)
	fmt.Println(none, one)
}
