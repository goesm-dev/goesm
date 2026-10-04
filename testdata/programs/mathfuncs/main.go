// Command mathfuncs checks the math functions goesm implements with JS
// builtins on special values: signed zeros, infinities and NaN.
package main

import (
	"fmt"
	"math"
)

func main() {
	vals := []float64{0, math.Copysign(0, -1), 1.5, -1.5, 2.5, -2.5, 1e300, -7.0000001, math.Inf(1), math.Inf(-1), math.NaN(), 0.49999999999999994}
	for _, v := range vals {
		bits := math.Float64bits(math.Abs(v))
		if math.IsNaN(v) {
			bits = 0 // JavaScriptCore (Bun) does not keep NaN payloads
		}
		fmt.Println(v, math.Floor(v), math.Ceil(v), math.Trunc(v), math.Round(v), math.RoundToEven(v), math.Sqrt(v), math.Abs(v), math.Signbit(v),
			math.Copysign(3, v), math.Signbit(math.Copysign(0, v)), math.Max(v, 0), math.Min(v, 0), bits)
	}
	fmt.Println(math.Inf(0), math.Inf(-3), math.IsNaN(math.NaN()), math.Sqrt(2), math.Sqrt(1e-310), math.Hypot(3, 4), math.Mod(7, -3), math.Pow(2, 0.5))
}
