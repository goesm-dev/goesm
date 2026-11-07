// Command sprintfconcat formats values that goesm's lowering of
// fmt.Sprintf with a constant format concatenates, at the edges where JS
// formatting differs from fmt's: ints and uints past 2^53, which fmt
// formats as their 64-bit conversions, a uint below zero, and floats that
// toFixed does not format as strconv does (large values, NaN and the
// infinities, many digits).
package main

import (
	"fmt"
	"math"
)

//go:noinline
func scale(n int, times int) int {
	for i := 0; i < times; i++ {
		n *= 1 << 20
	}
	return n
}

//go:noinline
func uscale(n uint, times int) uint {
	for i := 0; i < times; i++ {
		n *= 1 << 20
	}
	return n
}

func main() {
	ints := []int{0, -1, 42, 1<<53 - 1, -(1<<53 - 1), scale(1, 3), scale(-1, 3), scale(1<<10, 3), scale(-(1 << 5), 3), scale(3, 2) + 7}
	for _, n := range ints {
		fmt.Println(fmt.Sprintf("%d|%v|%x|%X", n, n, n, n))
	}
	var under uint = 3
	under -= 5
	uints := []uint{0, 7, 1<<53 - 1, uscale(1, 3), uscale(5, 3), under}
	for _, u := range uints {
		fmt.Println(fmt.Sprintf("%d|%v|%x|%X", u, u, u, u))
	}
	var p uintptr = 0xdeadbeef
	fmt.Println(fmt.Sprintf("p=%d %x", p, p))
	floats := []float64{0, math.Copysign(0, -1), 1.5, 2.675, -0.125, 1e20, 1e21, 1e22, -1e300, 123456789012345678901234567890.0, 5e-324, math.MaxFloat64, math.Inf(1), math.Inf(-1), math.NaN()}
	for _, f := range floats {
		fmt.Println(fmt.Sprintf("%f|%.2f|%.0f|%F|%.17f|%v", f, f, f, f, f, f))
	}
	fmt.Println(fmt.Sprintf("%.100f", 1.0/3))
	err := fmt.Errorf("n=%d f=%.1f", scale(1, 3), math.Inf(1))
	fmt.Println(err)
}
